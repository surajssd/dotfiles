// Package registry queries OCI/Docker container registries over HTTP to
// resolve the current digest of an image reference. It is intentionally
// dependency-free (uses only net/http) and tailored for the ghcr.io anonymous
// pull flow that clawbox's default image lives in.
package registry

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// defaultRegistry is used when an image reference has no registry component.
	defaultRegistry = "docker.io"
	// defaultTag is used when an image reference has no tag.
	defaultTag = "latest"

	// httpTimeout bounds each individual HTTP request. Combined with retries
	// this keeps the worst-case wall-clock for RemoteDigest small.
	httpTimeout = 5 * time.Second
)

// manifestAccept lists the media types we are willing to receive. The order
// is significant only for content negotiation; ghcr.io returns the same
// Docker-Content-Digest header regardless.
var manifestAccept = strings.Join([]string{
	"application/vnd.oci.image.index.v1+json",
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	"application/vnd.docker.distribution.manifest.v2+json",
}, ", ")

// retryDelays defines the backoff between attempts. len(retryDelays)+1 is the
// maximum total number of attempts. Kept short on purpose — failure should
// fall back to the local image quickly rather than block recreate.
var retryDelays = []time.Duration{
	500 * time.Millisecond,
	1 * time.Second,
}

// Ref describes a parsed image reference.
type Ref struct {
	Registry   string // e.g. "ghcr.io"
	Repository string // e.g. "surajssd/dotfiles/openclaw"
	Tag        string // e.g. "latest"
}

// ParseRef splits an image reference like "ghcr.io/foo/bar:tag" into its
// components. Missing registry defaults to docker.io; missing tag defaults
// to latest. Digest references (foo/bar@sha256:...) are not currently
// supported — the comparison only ever makes sense for tag-based refs.
func ParseRef(image string) (Ref, error) {
	if image == "" {
		return Ref{}, fmt.Errorf("empty image reference")
	}
	if strings.Contains(image, "@") {
		return Ref{}, fmt.Errorf("digest references are not supported: %s", image)
	}

	registry := defaultRegistry
	rest := image
	// A registry is present if the first path segment contains a '.' or ':'
	// (host[:port]). This is the standard heuristic used by Docker/containerd.
	if i := strings.Index(image, "/"); i > 0 {
		head := image[:i]
		if strings.ContainsAny(head, ".:") || head == "localhost" {
			registry = head
			rest = image[i+1:]
		}
	}

	tag := defaultTag
	if i := strings.LastIndex(rest, ":"); i >= 0 {
		tag = rest[i+1:]
		rest = rest[:i]
	}

	if rest == "" {
		return Ref{}, fmt.Errorf("empty repository in image reference: %s", image)
	}

	return Ref{
		Registry:   registry,
		Repository: rest,
		Tag:        tag,
	}, nil
}

// RemoteDigest returns the current Docker-Content-Digest of the image
// reference's tag as reported by the upstream registry. It retries a small
// number of times on transient failures; if every attempt fails it returns
// the last error so the caller can decide whether to proceed without the
// freshness check.
func RemoteDigest(image string) (string, error) {
	ref, err := ParseRef(image)
	if err != nil {
		return "", err
	}

	var lastErr error
	for attempt := 0; attempt <= len(retryDelays); attempt++ {
		if attempt > 0 {
			time.Sleep(retryDelays[attempt-1])
		}
		digest, err := fetchDigestOnce(ref)
		if err == nil {
			return digest, nil
		}
		lastErr = err
	}
	return "", lastErr
}

// fetchDigestOnce performs a single token+HEAD round trip.
func fetchDigestOnce(ref Ref) (string, error) {
	client := &http.Client{Timeout: httpTimeout}

	token, err := fetchAnonymousToken(client, ref)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", ref.Registry, ref.Repository, ref.Tag)
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return "", fmt.Errorf("building manifest request: %w", err)
	}
	req.Header.Set("Accept", manifestAccept)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("manifest request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("manifest HEAD %s: %s", url, resp.Status)
	}

	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("manifest HEAD %s: missing Docker-Content-Digest header", url)
	}
	return digest, nil
}

// fetchAnonymousToken obtains an anonymous bearer token for pulling the given
// repository. ghcr.io always requires a token even for public images; for
// other registries this returns an empty string when no auth challenge is
// needed.
func fetchAnonymousToken(client *http.Client, ref Ref) (string, error) {
	tokenURL, ok := tokenEndpoint(ref)
	if !ok {
		return "", nil
	}
	resp, err := client.Get(tokenURL)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("token request %s: %s", tokenURL, resp.Status)
	}

	// Both ghcr.io and docker.io return JSON like {"token":"...", "access_token":"..."}.
	// We parse minimally without pulling in a JSON dependency for one field.
	body := make([]byte, 0, 4096)
	buf := make([]byte, 1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			body = append(body, buf[:n]...)
			if len(body) > 64*1024 {
				return "", fmt.Errorf("token response too large")
			}
		}
		if rerr != nil {
			break
		}
	}

	if tok := extractJSONField(string(body), "token"); tok != "" {
		return tok, nil
	}
	if tok := extractJSONField(string(body), "access_token"); tok != "" {
		return tok, nil
	}
	return "", fmt.Errorf("token response missing token field")
}

// tokenEndpoint returns the anonymous-token URL for known registries.
func tokenEndpoint(ref Ref) (string, bool) {
	switch ref.Registry {
	case "ghcr.io":
		return fmt.Sprintf("https://ghcr.io/token?scope=repository:%s:pull", ref.Repository), true
	case "docker.io", "registry-1.docker.io":
		return fmt.Sprintf("https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull", ref.Repository), true
	}
	return "", false
}

// extractJSONField finds `"key":"value"` in a small JSON blob without
// pulling in encoding/json (and tolerates whitespace). It is intentionally
// minimal — the token endpoints used here return flat objects.
func extractJSONField(body, key string) string {
	needle := "\"" + key + "\""
	i := strings.Index(body, needle)
	if i < 0 {
		return ""
	}
	rest := body[i+len(needle):]
	// Skip past ':' and whitespace.
	j := strings.Index(rest, ":")
	if j < 0 {
		return ""
	}
	rest = rest[j+1:]
	rest = strings.TrimLeft(rest, " \t\n\r")
	if !strings.HasPrefix(rest, "\"") {
		return ""
	}
	rest = rest[1:]
	end := strings.Index(rest, "\"")
	if end < 0 {
		return ""
	}
	return rest[:end]
}
