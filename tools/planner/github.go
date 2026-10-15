package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

// prState is the state gh reports for one pull request URL, or the reason
// it could not.
type prState struct {
	state string
	err   error
}

func (s prState) String() string {
	if s.err != nil {
		return "state unknown: " + s.err.Error()
	}
	return s.state
}

const prStateWorkers = 4

// pullRequestURLs collects the distinct PullRequests entries of the plans.
func pullRequestURLs(plans []*plan) []string {
	var urls []string
	for _, p := range plans {
		if p.valid() {
			urls = append(urls, p.front.PullRequests...)
		}
	}
	return mergeLinks(nil, urls)
}

// fetchPRStates asks gh for the state of every URL, a few at a time.
func fetchPRStates(urls []string) map[string]prState {
	states := make(map[string]prState, len(urls))
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, prStateWorkers)
	for _, url := range urls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			state := ghPRState(url)
			mu.Lock()
			states[url] = state
			mu.Unlock()
		}()
	}
	wg.Wait()
	return states
}

// ghPRState runs gh pr view for one URL. The first line gh prints on stderr
// becomes the reason when it fails, so an authentication problem is visible.
func ghPRState(url string) prState {
	ctx, cancel := context.WithTimeout(context.Background(), ghTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", url, "--json", "state")
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		reason, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
		if reason == "" {
			reason = err.Error()
		}
		return prState{err: errors.New(reason)}
	}
	var view struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(output, &view); err != nil || view.State == "" {
		return prState{err: errors.New("gh printed no state")}
	}
	return prState{state: view.State}
}

var (
	prNumberRE   = regexp.MustCompile(`/pull/([0-9]+)/?$`)
	hashNumberRE = regexp.MustCompile(`#([0-9]+)`)
	sentenceRE   = regexp.MustCompile(`[.!?](\s+|$)`)
	openWordRE   = regexp.MustCompile(`(?i)\bopen\b`)
)

// githubFindings applies the rules that need GitHub: pr-state-unknown for a
// URL gh could not answer, and stale-pr-note for a pull request the
// StatusNote calls open while gh says CLOSED or MERGED. A sentence names a
// pull request by its URL or by #<number>; when several listed pull requests
// share a number, it counts as open while any of them is open or unknown.
func githubFindings(plans []*plan, states map[string]prState) []finding {
	var findings []finding
	for _, p := range plans {
		if !p.valid() {
			continue
		}
		urls := mergeLinks(nil, p.front.PullRequests)
		openNumbers := map[string]bool{}
		for _, url := range urls {
			if s := states[url]; s.err != nil {
				findings = append(findings, finding{plan: p, rule: "pr-state-unknown", severity: advisory, detail: url + ": " + s.err.Error()})
			}
			if s := states[url]; s.err != nil || s.state == "OPEN" {
				if m := prNumberRE.FindStringSubmatch(url); m != nil {
					openNumbers[m[1]] = true
				}
			}
		}
		sentences := sentenceRE.Split(p.front.StatusNote, -1)
		for _, url := range urls {
			s := states[url]
			if s.err != nil || s.state == "OPEN" {
				continue
			}
			number := ""
			if m := prNumberRE.FindStringSubmatch(url); m != nil {
				number = m[1]
			}
			for _, sentence := range sentences {
				if !openWordRE.MatchString(sentence) || !mentionsPullRequest(sentence, url, number, openNumbers) {
					continue
				}
				label := url
				if number != "" {
					label = "#" + number
				}
				findings = append(findings, finding{plan: p, rule: "stale-pr-note", severity: advisory, detail: fmt.Sprintf("StatusNote calls %s open; GitHub says %s", label, s.state)})
				break
			}
		}
	}
	return findings
}

func mentionsPullRequest(sentence, url, number string, openNumbers map[string]bool) bool {
	if strings.Contains(sentence, url) {
		return true
	}
	if number == "" || openNumbers[number] {
		return false
	}
	for _, m := range hashNumberRE.FindAllStringSubmatch(sentence, -1) {
		if m[1] == number {
			return true
		}
	}
	return false
}
