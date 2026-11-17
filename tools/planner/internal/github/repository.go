package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const ghTimeout = 10 * time.Second

var (
	sshURLRE = regexp.MustCompile(`^ssh://(?:[^/@\s]+@)?github\.com(?::[0-9]+)?/(.+)$`)
	scpURLRE = regexp.MustCompile(`^[^/@:\s]+@github\.com:(.+)$`)
	repoRE   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9._-]+$`)
)

func validGithubRepo(repo string) bool {
	if !repoRE.MatchString(repo) {
		return false
	}
	name := repo[strings.LastIndex(repo, "/")+1:]
	return name != "." && name != ".."
}

func githubRepoFromURL(url string) (string, bool) {
	url = strings.TrimSpace(url)
	var repo string
	switch {
	case strings.HasPrefix(url, "https://github.com/"):
		repo = strings.TrimPrefix(url, "https://github.com/")
	case sshURLRE.MatchString(url):
		repo = sshURLRE.FindStringSubmatch(url)[1]
	case scpURLRE.MatchString(url):
		repo = scpURLRE.FindStringSubmatch(url)[1]
	default:
		return "", false
	}
	repo = strings.TrimSuffix(strings.TrimSuffix(repo, "/"), ".git")
	if !validGithubRepo(repo) {
		return "", false
	}
	return repo, true
}

// ResolveRepository identifies the GitHub repository of the current checkout: the
// upstream remote, then the origin remote mapped through gh to its fork
// parent, then origin itself with a warning.
func ResolveRepository(stderr io.Writer) (string, error) {
	inside, err := exec.Command("git", "rev-parse", "--is-inside-work-tree").Output()
	if err != nil || strings.TrimSpace(string(inside)) != "true" {
		return "", errors.New("run planner create from a Git checkout or worktree")
	}
	if url, err := exec.Command("git", "remote", "get-url", "upstream").Output(); err == nil {
		if repo, ok := githubRepoFromURL(string(url)); ok {
			return repo, nil
		}
	}
	url, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", errors.New("could not resolve a github.com repository from the upstream or origin remote")
	}
	origin, ok := githubRepoFromURL(string(url))
	if !ok {
		return "", errors.New("could not resolve a github.com repository from the upstream or origin remote")
	}
	if repo, ok := lookupForkParent(origin); ok {
		return repo, nil
	}
	_, _ = fmt.Fprintf(stderr, "warning: GitHub lookup failed or is unavailable; using origin (%s); fork parent could not be verified\n", origin)
	return origin, nil
}

func lookupForkParent(origin string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), ghTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "repo", "view", "https://github.com/"+origin, "--json", "isFork,parent,nameWithOwner")
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1")
	output, err := cmd.Output()
	if err != nil {
		return "", false
	}
	var view struct {
		IsFork *bool `json:"isFork"`
		Parent struct {
			Owner struct {
				Login string `json:"login"`
			} `json:"owner"`
			Name string `json:"name"`
		} `json:"parent"`
		NameWithOwner string `json:"nameWithOwner"`
	}
	if err := json.Unmarshal(output, &view); err != nil || view.IsFork == nil {
		return "", false
	}
	repo := view.NameWithOwner
	if *view.IsFork {
		repo = view.Parent.Owner.Login + "/" + view.Parent.Name
	}
	if !validGithubRepo(repo) {
		return "", false
	}
	return repo, true
}
