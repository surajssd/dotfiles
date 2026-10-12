package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/oauth2"
)

const credentialService = "YT_PLAYLIST_CREATOR_OAUTH"
const youtubeScope = "https://www.googleapis.com/auth/youtube.force-ssl"

type credentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
}

func parseCredentials(data []byte) (credentials, error) {
	var bundle credentials
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if len(data) > 64<<10 || !utf8.Valid(data) || dec.Decode(&bundle) != nil {
		return bundle, fmt.Errorf("invalid credential bundle; run yt-playlist-creator auth login again")
	}
	if dec.Decode(new(any)) != io.EOF || strings.TrimSpace(bundle.ClientID) == "" || strings.TrimSpace(bundle.ClientSecret) == "" || strings.TrimSpace(bundle.RefreshToken) == "" {
		return credentials{}, fmt.Errorf("credential bundle needs client_id, client_secret, and refresh_token; run yt-playlist-creator auth login again")
	}
	return bundle, nil
}

func loadCredentials(ctx context.Context, deps dependencies) (credentials, error) {
	if err := ctx.Err(); err != nil {
		return credentials{}, err
	}
	if value := os.Getenv(credentialService); value != "" {
		bundle, err := parseCredentials([]byte(value))
		if err != nil {
			return credentials{}, fmt.Errorf("%s: %w", credentialService, err)
		}
		return bundle, nil
	}
	if deps.platform != "darwin" {
		return credentials{}, fmt.Errorf("set %s to a provisioned credential bundle; auth login requires macOS", credentialService)
	}
	security, err := exec.LookPath("security")
	if err != nil {
		return credentials{}, fmt.Errorf("security was not found; run auth login on macOS or set %s", credentialService)
	}
	account, err := keychainAccount()
	if err != nil {
		return credentials{}, err
	}
	data, err := runSecurity(ctx, deps, security, nil, "find-generic-password", "-a", account, "-s", credentialService, "-w")
	if err != nil {
		return credentials{}, fmt.Errorf("read Keychain credentials: %w; run yt-playlist-creator auth login", err)
	}
	return parseCredentials(keychainValue(data))
}

func keychainValue(data []byte) []byte {
	data = bytes.TrimSpace(data)
	if decoded, err := hex.DecodeString(string(data)); err == nil {
		return decoded
	}
	return data
}

func saveCredentials(ctx context.Context, deps dependencies, security, account string, bundle credentials) error {
	data, err := json.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("cannot encode credentials for Keychain")
	}
	quote := func(value string) string {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
	}
	// security -i has a 4096-byte line buffer; -X avoids its password prompt and JSON quoting.
	command := "add-generic-password -U -a " + quote(account) + " -s " + credentialService + " -D \"api key\" -X " + hex.EncodeToString(data) + "\n"
	if len(command) >= 4095 {
		return fmt.Errorf("credential bundle exceeds the Keychain command limit; existing credentials were not changed")
	}
	if _, err := runSecurity(ctx, deps, security, strings.NewReader(command), "-i"); err != nil {
		return fmt.Errorf("save Keychain credentials: %w", err)
	}
	stored, err := runSecurity(ctx, deps, security, nil, "find-generic-password", "-a", account, "-s", credentialService, "-w")
	if err != nil || !bytes.Equal(keychainValue(stored), data) {
		return fmt.Errorf("could not verify the saved Keychain credentials; check Keychain before running auth login again")
	}
	return nil
}

func keychainAccount() (string, error) {
	account := os.Getenv("USER")
	if strings.TrimSpace(account) == "" || strings.ContainsAny(account, "\x00\r\n") || !utf8.ValidString(account) {
		return "", fmt.Errorf("USER must name the Keychain account")
	}
	return account, nil
}

func oauthConfig(deps dependencies, bundle credentials) oauth2.Config {
	endpoint := deps.oauthEndpoint
	endpoint.AuthStyle = oauth2.AuthStyleInParams
	return oauth2.Config{
		ClientID: bundle.ClientID, ClientSecret: bundle.ClientSecret,
		Endpoint: endpoint, Scopes: []string{youtubeScope},
	}
}

func oauthContext(ctx context.Context, deps dependencies) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, boundedHTTPClient(deps.httpClient))
}

func authLogin(ctx context.Context, deps dependencies, path string) error {
	if path == "" {
		return fmt.Errorf("--client-secrets is required")
	}
	if deps.platform != "darwin" {
		return fmt.Errorf("auth login requires macOS Keychain; use a provisioned %s bundle on this host", credentialService)
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open client secrets: %w", err)
	}
	defer func() { _ = file.Close() }()
	data, err := readBounded(ctx, file, 1<<20)
	if err != nil {
		return fmt.Errorf("read client secrets: %w", err)
	}
	var client struct {
		Installed *struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		} `json:"installed"`
	}
	if !utf8.Valid(data) || json.Unmarshal(data, &client) != nil || client.Installed == nil || strings.TrimSpace(client.Installed.ClientID) == "" || strings.TrimSpace(client.Installed.ClientSecret) == "" {
		return fmt.Errorf("client secrets must contain an installed Desktop app with client_id and client_secret")
	}
	security, err := exec.LookPath("security")
	if err != nil {
		return fmt.Errorf("security was not found; install or enable macOS Keychain before auth login")
	}
	account, err := keychainAccount()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, deps.authTimeout)
	defer cancel()
	listener, err := deps.listen(ctx)
	if err != nil {
		return fmt.Errorf("cannot listen on the loopback interface for authorization")
	}
	defer func() { _ = listener.Close() }()
	bundle := credentials{ClientID: client.Installed.ClientID, ClientSecret: client.Installed.ClientSecret}
	config := oauthConfig(deps, bundle)
	config.RedirectURL = "http://" + listener.Addr().String() + "/oauth2/callback"
	state, verifier := oauth2.GenerateVerifier(), oauth2.GenerateVerifier()
	type callback struct {
		code string
		err  error
	}
	callbacks := make(chan callback, 1)
	var once sync.Once
	server := &http.Server{
		ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second,
		MaxHeaderBytes: 8192, ErrorLog: log.New(io.Discard, "", 0),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			if r.URL.Path != "/oauth2/callback" || r.Host != listener.Addr().String() {
				http.NotFound(w, r)
				return
			}
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			values, err := url.ParseQuery(r.URL.RawQuery)
			result := callback{}
			switch {
			case err != nil || len(values["state"]) != 1 || subtle.ConstantTimeCompare([]byte(values.Get("state")), []byte(state)) != 1:
				result.err = fmt.Errorf("authorization callback has an invalid state")
			case len(values["error"]) != 0:
				result.err = fmt.Errorf("authorization was denied or failed; run auth login again")
			case len(values["code"]) != 1 || strings.TrimSpace(values.Get("code")) == "":
				result.err = fmt.Errorf("authorization callback has no single authorization code")
			default:
				result.code = values.Get("code")
			}
			if result.err != nil {
				http.Error(w, "Authorization failed. Return to the terminal.", http.StatusBadRequest)
			} else {
				_, _ = fmt.Fprintln(w, "Authorization received. Return to the terminal.")
			}
			once.Do(func() { callbacks <- result })
		}),
	}
	defer func() { _ = server.Close() }()
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve(listener) }()
	authorizationURL := config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce, oauth2.S256ChallengeOption(verifier))
	if _, err := fmt.Fprintln(deps.stderr, "Open this URL in a browser on this machine:\n"+authorizationURL); err != nil {
		return fmt.Errorf("cannot print the authorization URL")
	}
	if deps.authorizationURL != nil {
		if err := deps.authorizationURL(authorizationURL); err != nil {
			return fmt.Errorf("cannot deliver the authorization URL")
		}
	}
	var code string
	select {
	case <-ctx.Done():
		return fmt.Errorf("authorization did not finish: %w", ctx.Err())
	case <-serveErrors:
		return fmt.Errorf("authorization callback listener stopped")
	case result := <-callbacks:
		if result.err != nil {
			return result.err
		}
		code = result.code
	}
	token, err := config.Exchange(oauthContext(ctx, deps), code, oauth2.VerifierOption(verifier))
	if err != nil {
		return oauthError(ctx, err)
	}
	if strings.TrimSpace(token.RefreshToken) == "" {
		return fmt.Errorf("authorization returned no refresh token; run auth login again and grant offline access")
	}
	bundle.RefreshToken = token.RefreshToken
	if err := saveCredentials(ctx, deps, security, account, bundle); err != nil {
		return err
	}
	_, err = fmt.Fprintln(deps.stderr, "Credentials saved in macOS Keychain.")
	return err
}

func oauthError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("authorization: %w", ctx.Err())
	}
	var retrieve *oauth2.RetrieveError
	if errors.As(err, &retrieve) {
		switch retrieve.ErrorCode {
		case "invalid_grant", "invalid_token":
			return fmt.Errorf("authorization expired or was revoked; run yt-playlist-creator auth login again")
		case "invalid_scope", "insufficient_scope", "access_denied":
			return fmt.Errorf("authorization lacks the YouTube permission; run yt-playlist-creator auth login again")
		case "invalid_client", "unauthorized_client":
			return fmt.Errorf("OAuth client was rejected; check the Desktop app client and run auth login again")
		}
		if retrieve.Response != nil {
			return fmt.Errorf("authorization failed (HTTP %d); run yt-playlist-creator auth login again", retrieve.Response.StatusCode)
		}
	}
	return fmt.Errorf("authorization request failed or returned an invalid token; run yt-playlist-creator auth login again")
}

type securityOutput struct {
	buffer bytes.Buffer
}

func (b *securityOutput) Write(data []byte) (int, error) {
	if b.buffer.Len()+len(data) > 64<<10 {
		return 0, fmt.Errorf("security output exceeded the size limit")
	}
	return b.buffer.Write(data)
}

func runSecurity(ctx context.Context, deps dependencies, binary string, stdin io.Reader, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, deps.securityTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdin = stdin
	cmd.WaitDelay = time.Second
	var stdout securityOutput
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("security did not finish: %w", ctx.Err())
		}
		return nil, fmt.Errorf("security failed; check that Keychain is unlocked and allows access")
	}
	return stdout.buffer.Bytes(), nil
}
