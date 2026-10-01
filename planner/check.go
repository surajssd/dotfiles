package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

var statusValues = []string{
	"NotImplemented",
	"InProgress",
	"PartiallyImplemented",
	"ImplementedUnmerged",
	"Implemented",
	"Superseded",
}

// canonicalStatus maps a status typed on the command line to its recognised
// spelling, ignoring case and hyphens, so in-progress becomes InProgress.
func canonicalStatus(input string) (string, bool) {
	wanted := strings.ReplaceAll(strings.ToLower(input), "-", "")
	for _, value := range statusValues {
		if strings.ToLower(value) == wanted {
			return value, true
		}
	}
	return "", false
}

const checkHelp = `Report problems in the plans under the root as a table with one row per
finding: FILE, REPO, SEVERITY, RULE, and DETAIL, sorted by file, then rule.
The exit status is 1 when any error finding exists and 0 when there are only
advisories or no findings.

With plan names (a path, a [[wikilink]], a basename, the short NAME shown by
planner get, or a URL the plan lists) only the findings of those plans are
shown. A name that matches no plan is reported after the table and the exit
status is 1.

--github asks gh pr view for the state of every pull request the plans in
scope list (the active plans, or the named ones) and adds the two rules
marked below. It needs gh on PATH, logged in, and network access.

Front matter keys: Type, ImplementationStatus, StatusChecked, and
StatusNote are required; Parent, SupersededBy, Issues, and PullRequests
are optional. Parent and SupersededBy hold a "[[wikilink]]", a path, or a
URL; Issues and PullRequests hold lists of URLs.

Recognised ImplementationStatus values:

  NotImplemented, InProgress, PartiallyImplemented, ImplementedUnmerged,
  Implemented, Superseded

Rules:

  no-front-matter       advisory  the first line is not ---
  invalid-front-matter  error     no closing --- or the YAML does not decode
  missing-field         error     Type, ImplementationStatus, StatusChecked,
                                  or StatusNote is absent or empty
  unknown-key           advisory  a front matter key outside the schema
  unknown-status        error     ImplementationStatus is not a recognised value
  invalid-date          error     StatusChecked is not YYYY-MM-DD
  invalid-link          error     an Issues or PullRequests entry is not an
                                  http(s) URL, a github.com PullRequests entry
                                  is not a pull request, or a github.com Issues
                                  entry is a pull request
  parent-not-found      error     a wikilink parent matches zero or several
                                  plans, or a path parent does not exist
  parent-cycle          error     following parent links returns to the plan
  successor-not-found   error     the same for SupersededBy
  missing-successor     advisory  ImplementationStatus is Superseded but
                                  SupersededBy is empty
  dead-link             error     a link in the body or in StatusNote names a
                                  plan-style file (twelve digits, a hyphen, a
                                  name) that is not under the root
  undumped-reference    advisory  Parent, SupersededBy, or a link in the body
                                  or in StatusNote points into .claude/plans/
  duplicate-title       advisory  two or more plans in one repository folder
                                  share an H1 without replacing one another
                                  through SupersededBy
  stale-pr-note         advisory  (--github) a sentence of StatusNote calls a
                                  listed pull request open, by URL or #<n>,
                                  while GitHub says CLOSED or MERGED
  pr-state-unknown      advisory  (--github) gh could not report the state of
                                  a listed pull request URL`

type severity int

const (
	advisory severity = iota
	failure
)

type finding struct {
	plan     *plan
	rule     string
	detail   string
	severity severity
}

func (s severity) String() string {
	if s == failure {
		return "error"
	}
	return "advisory"
}

type checkOptions struct {
	github bool
}

func runCheck(deps dependencies, root string, names []string, opts checkOptions) error {
	c, err := loadCorpus(root)
	if err != nil {
		return err
	}
	findings := checkCorpus(c)
	var scope []*plan
	var missing error
	if len(names) > 0 {
		scope, missing = findPlans(c, names)
		if len(scope) == 0 {
			return missing
		}
		findings = findingsFor(findings, scope)
	} else {
		for _, p := range c.plans {
			if p.valid() && p.active() {
				scope = append(scope, p)
			}
		}
	}
	if opts.github {
		states := fetchPRStates(pullRequestURLs(scope))
		findings = sortFindings(append(findings, githubFindings(scope, states)...))
	}
	if len(findings) == 0 {
		if err := writeOutput(deps.stderr, "No findings.\n"); err != nil {
			return err
		}
		return missing
	}
	table := [][]string{{"FILE", "REPO", "SEVERITY", "RULE", "DETAIL"}}
	errorCount, advisoryCount := 0, 0
	for _, f := range findings {
		repo := f.plan.repo
		if repo == "" {
			repo = emptyCell
		}
		table = append(table, []string{filepath.Base(f.plan.relPath), repo, f.severity.String(), f.rule, f.detail})
		if f.severity == failure {
			errorCount++
		} else {
			advisoryCount++
		}
	}
	if err := writeOutput(deps.stdout, renderTable(table, 0)); err != nil {
		return err
	}
	if errorCount > 0 {
		summary := fmt.Errorf("check: %s, %s", plural(errorCount, "error", "errors"), plural(advisoryCount, "advisory", "advisories"))
		if missing != nil {
			return errors.Join(missing, summary)
		}
		return summary
	}
	return missing
}

// findingsFor keeps the findings that belong to the given plans, in order.
func findingsFor(findings []finding, plans []*plan) []finding {
	wanted := map[*plan]bool{}
	for _, p := range plans {
		wanted[p] = true
	}
	var kept []finding
	for _, f := range findings {
		if wanted[f.plan] {
			kept = append(kept, f)
		}
	}
	return kept
}

func checkCorpus(c *corpus) []finding {
	var findings []finding
	add := func(p *plan, rule string, sev severity, detail string) {
		findings = append(findings, finding{plan: p, rule: rule, detail: detail, severity: sev})
	}
	for _, p := range c.plans {
		switch {
		case !p.hasFront:
			add(p, "no-front-matter", advisory, "first line is not ---")
		case p.frontErr != nil:
			add(p, "invalid-front-matter", failure, strings.Join(strings.Fields(p.frontErr.Error()), " "))
		default:
			checkFields(c, p, add)
		}
		checkLinks(c, p, add)
	}
	checkDuplicateTitles(c, add)
	return sortFindings(findings)
}

// sortFindings orders findings by file, then rule, then detail.
func sortFindings(findings []finding) []finding {
	sort.Slice(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.plan.relPath != b.plan.relPath {
			return a.plan.relPath < b.plan.relPath
		}
		if a.rule != b.rule {
			return a.rule < b.rule
		}
		return a.detail < b.detail
	})
	return findings
}

type addFunc func(p *plan, rule string, sev severity, detail string)

func checkFields(c *corpus, p *plan, add addFunc) {
	fields := []struct{ name, value string }{
		{"Type", p.front.Type},
		{"ImplementationStatus", p.front.ImplementationStatus},
		{"StatusChecked", p.front.StatusChecked},
		{"StatusNote", p.front.StatusNote},
	}
	for _, f := range fields {
		if strings.TrimSpace(f.value) == "" {
			add(p, "missing-field", failure, f.name)
		}
	}
	for _, key := range p.unknownKeys {
		add(p, "unknown-key", advisory, key)
	}
	if status := p.front.ImplementationStatus; status != "" && !slices.Contains(statusValues, status) {
		add(p, "unknown-status", failure, fmt.Sprintf("ImplementationStatus %q is not one of %s", status, strings.Join(statusValues, ", ")))
	}
	if checked := strings.TrimSpace(p.front.StatusChecked); checked != "" {
		if _, err := time.Parse("2006-01-02", checked); err != nil {
			add(p, "invalid-date", failure, fmt.Sprintf("StatusChecked %q is not YYYY-MM-DD", checked))
		}
	}
	checkReference(c, p, "Parent", p.front.Parent, "parent-not-found", add)
	checkReference(c, p, "SupersededBy", p.front.SupersededBy, "successor-not-found", add)
	if p.front.ImplementationStatus == "Superseded" && strings.TrimSpace(p.front.SupersededBy) == "" {
		add(p, "missing-successor", advisory, "ImplementationStatus is Superseded but SupersededBy is empty")
	}
	for _, key := range []string{issuesKey, pullRequestsKey} {
		for _, value := range linksFor(p.front, key) {
			if problem := linkProblem(key, value); problem != "" {
				add(p, "invalid-link", failure, problem)
			}
		}
	}
}

// checkReference validates a Parent or SupersededBy value: a wikilink must
// match exactly one plan, a path must exist, a URL is taken as is. The cycle
// check applies to parent only, because only parent links form the tree.
func checkReference(c *corpus, p *plan, key, value, rule string, add addFunc) {
	if isUndumpedReference(value) {
		add(p, "undumped-reference", advisory, key+": "+value)
	}
	switch classifyParent(value) {
	case parentNone, parentURL:
		return
	case parentWikilink:
		matches := c.lookup(reduceLinkTarget(value))
		switch len(matches) {
		case 1:
			if key != "Parent" {
				return
			}
			if chain, cycle := parentChain(c, p); cycle {
				add(p, "parent-cycle", failure, strings.Join(chain, " -> "))
			}
		case 0:
			add(p, rule, failure, fmt.Sprintf("no plan matches %s", value))
		default:
			var paths []string
			for _, m := range matches {
				paths = append(paths, m.relPath)
			}
			add(p, rule, failure, fmt.Sprintf("%d plans match %s: %s", len(matches), value, strings.Join(paths, ", ")))
		}
	case parentPath:
		resolved, err := expandParentPath(value, filepath.Dir(p.path))
		if err != nil {
			add(p, rule, failure, fmt.Sprintf("%s: %v", value, err))
			return
		}
		_, err = os.Stat(resolved)
		switch {
		case errors.Is(err, fs.ErrNotExist) && strings.HasPrefix(value, "."):
			add(p, rule, failure, fmt.Sprintf("%s does not exist (resolved to %s)", value, resolved))
		case errors.Is(err, fs.ErrNotExist):
			add(p, rule, failure, fmt.Sprintf("%s does not exist", value))
		case err != nil:
			add(p, rule, failure, fmt.Sprintf("%s: %v", value, err))
		}
	default:
		add(p, rule, failure, fmt.Sprintf("unrecognised %s form %q (expected \"[[plan]]\", a path, or a URL)", key, value))
	}
}

// checkLinks applies the link rules to the body and to StatusNote. A note
// finding names its source, because the note is not visible in the body.
func checkLinks(c *corpus, p *plan, add addFunc) {
	seen := map[string]bool{}
	check := func(link, source string) {
		if seen[link] {
			return
		}
		seen[link] = true
		if isUndumpedReference(link) {
			add(p, "undumped-reference", advisory, source+link)
		}
		basename := reduceLinkTarget(link)
		if isPlanStyle(basename) && len(c.lookup(basename)) == 0 {
			add(p, "dead-link", failure, source+link)
		}
	}
	for _, link := range p.links {
		check(link, "")
	}
	for _, link := range p.noteLinks {
		check(link, "StatusNote: ")
	}
}

func checkDuplicateTitles(c *corpus, add addFunc) {
	type key struct{ dir, title string }
	groups := map[key][]*plan{}
	var order []key
	for _, p := range c.plans {
		if !p.hasTitle {
			continue
		}
		k := key{dir: filepath.Dir(p.relPath), title: p.title}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], p)
	}
	for _, k := range order {
		var group []*plan
		for _, p := range groups[k] {
			if !supersededInTitleGroup(c, p, groups[k]) {
				group = append(group, p)
			}
		}
		if len(group) < 2 {
			continue
		}
		var others []string
		for _, p := range group[1:] {
			others = append(others, filepath.Base(p.relPath))
		}
		add(group[0], "duplicate-title", advisory, fmt.Sprintf("%q is also the title of %s", k.title, strings.Join(others, ", ")))
	}
}

func supersededInTitleGroup(c *corpus, p *plan, group []*plan) bool {
	current := p
	seen := map[*plan]bool{p: true}
	for current.valid() && current.front.ImplementationStatus == "Superseded" {
		var next *plan
		value := current.front.SupersededBy
		switch classifyParent(value) {
		case parentWikilink:
			if matches := c.lookup(reduceLinkTarget(value)); len(matches) == 1 {
				next = matches[0]
			}
		case parentPath:
			path, err := expandParentPath(value, filepath.Dir(current.path))
			if err == nil {
				if resolved, err := filepath.EvalSymlinks(path); err == nil {
					next = c.byPath[resolved]
				}
			}
		}
		if next == nil || !next.valid() || !slices.Contains(group, next) {
			break
		}
		if seen[next] {
			return false
		}
		seen[next] = true
		current = next
	}
	return current != p
}
