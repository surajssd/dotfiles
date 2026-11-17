package cli

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/surajssd/dotfiles/tools/planner/internal/github"
	"github.com/surajssd/dotfiles/tools/planner/internal/plan"
)

// pullRequestURLs collects the distinct PullRequests entries of the plans.
func pullRequestURLs(plans []*plan.Plan) []string {
	var urls []string
	for _, p := range plans {
		if p.Valid() {
			urls = append(urls, p.Front.PullRequests...)
		}
	}
	return plan.MergeLinks(nil, urls)
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
func githubFindings(plans []*plan.Plan, states map[string]github.PRState) []plan.Finding {
	var findings []plan.Finding
	for _, p := range plans {
		if !p.Valid() {
			continue
		}
		urls := plan.MergeLinks(nil, p.Front.PullRequests)
		openNumbers := map[string]bool{}
		for _, url := range urls {
			if s := states[url]; s.Err != nil {
				findings = append(findings, plan.Finding{Plan: p, Rule: "pr-state-unknown", Severity: plan.Advisory, Detail: url + ": " + s.Err.Error()})
			}
			if s := states[url]; s.Err != nil || s.State == "OPEN" {
				if m := prNumberRE.FindStringSubmatch(url); m != nil {
					openNumbers[m[1]] = true
				}
			}
		}
		sentences := sentenceRE.Split(p.Front.StatusNote, -1)
		for _, url := range urls {
			s := states[url]
			if s.Err != nil || s.State == "OPEN" {
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
				findings = append(findings, plan.Finding{Plan: p, Rule: "stale-pr-note", Severity: plan.Advisory, Detail: fmt.Sprintf("StatusNote calls %s open; GitHub says %s", label, s.State)})
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
