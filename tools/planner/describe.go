package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// runDescribe prints one block per named plan. A name that resolves to no
// plan is reported after the blocks of the plans that were found.
func runDescribe(deps dependencies, root string, names []string, github bool) error {
	c, err := loadCorpus(root)
	if err != nil {
		return err
	}
	_, all := buildForest(c)
	children := map[*plan][]*plan{}
	for _, n := range all {
		if n.parent != nil && n.parent.plan != nil {
			children[n.parent.plan] = append(children[n.parent.plan], n.plan)
		}
	}
	findings := checkCorpus(c)
	home, _ := os.UserHomeDir()
	plans, missing := findPlans(c, names)
	var states map[string]prState
	if github {
		states = fetchPRStates(pullRequestURLs(plans))
		findings = sortFindings(append(findings, githubFindings(plans, states)...))
	}
	var out strings.Builder
	for i, p := range plans {
		if i > 0 {
			out.WriteString("\n\n")
		}
		describePlan(&out, p, children[p], findings, states, home, deps.now())
	}
	if err := writeOutput(deps.stdout, out.String()); err != nil {
		return err
	}
	return missing
}

// describePlan writes one kubectl describe style block: aligned Key: Value
// lines, with list values on indented lines below their key. With states,
// each pull request URL is followed by what GitHub reports for it.
func describePlan(out *strings.Builder, p *plan, children []*plan, findings []finding, states map[string]prState, home string, now time.Time) {
	field := func(key, value string) {
		if value == "" {
			value = emptyCell
		}
		fmt.Fprintf(out, "%-16s%s\n", key+":", value)
	}
	list := func(key string, items []string) {
		if len(items) == 0 {
			field(key, emptyCell)
			return
		}
		fmt.Fprintf(out, "%s:\n", key)
		for _, item := range items {
			fmt.Fprintf(out, "  %s\n", item)
		}
	}
	field("Name", p.name)
	field("Title", p.title)
	field("File", displayPath(p.path, home))
	field("Repo", p.repo)
	field("Type", p.front.Type)
	field("Status", p.front.ImplementationStatus)
	checked := ""
	if strings.TrimSpace(p.front.StatusChecked) != "" {
		checked = fmt.Sprintf("%s (%s)", strings.TrimSpace(p.front.StatusChecked), checkedText(p, now))
	}
	field("Checked", checked)
	field("Parent", p.front.Parent)
	field("Superseded By", p.front.SupersededBy)
	var names []string
	for _, child := range children {
		names = append(names, child.name)
	}
	list("Children", names)
	list("Issues", p.front.Issues)
	list("Pull Requests", pullRequestLines(p.front.PullRequests, states))
	field("Note", strings.TrimSpace(p.front.StatusNote))
	var problems []string
	for _, f := range findings {
		if f.plan == p {
			problems = append(problems, fmt.Sprintf("%-9s %-21s %s", f.severity.String(), f.rule, f.detail))
		}
	}
	list("Findings", problems)
}

// pullRequestLines pads the URLs to one width and appends the state GitHub
// reports, or returns them as written when no states were fetched.
func pullRequestLines(urls []string, states map[string]prState) []string {
	if states == nil {
		return urls
	}
	width := 0
	for _, url := range urls {
		width = max(width, len(strings.TrimSpace(url)))
	}
	var lines []string
	for _, url := range urls {
		url = strings.TrimSpace(url)
		lines = append(lines, fmt.Sprintf("%-*s  %s", width, url, states[url]))
	}
	return lines
}
