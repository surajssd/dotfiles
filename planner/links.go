package main

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const (
	issuesKey       = "issues"
	pullRequestsKey = "pull_requests"
)

var githubItemRE = regexp.MustCompile(`^/[^/]+/[^/]+/(pull|issues)/[0-9]+/?$`)

// linkProblem explains why value does not belong under key, or returns "".
// Every entry must be an http(s) URL. On github.com the path also has to
// match the key: a pull request under pull_requests, anything but a pull
// request under issues. Other hosts are accepted as they are, because a
// tracker URL has no shape the tool can check.
func linkProblem(key, value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return fmt.Sprintf("%s: %q is not an http(s) URL", key, value)
	}
	if parsed.Host != "github.com" {
		return ""
	}
	match := githubItemRE.FindStringSubmatch(parsed.Path)
	switch {
	case key == pullRequestsKey && (match == nil || match[1] != "pull"):
		return fmt.Sprintf("%s: %s is not a GitHub pull request URL", key, value)
	case key == issuesKey && match != nil && match[1] == "pull":
		return fmt.Sprintf("%s: %s is a pull request URL; put it under %s", key, value, pullRequestsKey)
	}
	return ""
}

// mergeLinks appends the new values that are not already present, trimmed,
// keeping the existing order.
func mergeLinks(existing, added []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range append(append([]string{}, existing...), added...) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// listLines renders a front matter list as a block sequence without a
// trailing newline. URLs need no quoting.
func listLines(key string, values []string) string {
	lines := []string{key + ":"}
	for _, value := range values {
		lines = append(lines, "  - "+value)
	}
	return strings.Join(lines, "\n")
}

// linksFor returns the list stored under key.
func linksFor(front frontMatter, key string) []string {
	if key == pullRequestsKey {
		return front.PullRequests
	}
	return front.Issues
}

// linkLabel is the describe heading for a list key.
func linkLabel(key string) string {
	if key == pullRequestsKey {
		return "Pull Requests"
	}
	return "Issues"
}
