package plan

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

func StatusValues() []string {
	return []string{
		"NotImplemented",
		"InProgress",
		"PartiallyImplemented",
		"ImplementedUnmerged",
		"Implemented",
		"Superseded",
		"Abandoned",
	}
}

// CanonicalStatus maps a status typed on the command line to its recognised
// spelling, ignoring case and hyphens, so in-progress becomes InProgress.
func CanonicalStatus(input string) (string, bool) {
	wanted := strings.ReplaceAll(strings.ToLower(input), "-", "")
	for _, value := range StatusValues() {
		if strings.ToLower(value) == wanted {
			return value, true
		}
	}
	return "", false
}

type Severity int

const (
	Advisory Severity = iota
	Failure
)

type Finding struct {
	Plan     *Plan
	Rule     string
	Detail   string
	Severity Severity
}

func (s Severity) String() string {
	if s == Failure {
		return "error"
	}
	return "advisory"
}

// FindingsFor keeps the findings that belong to the given plans, in order.
func FindingsFor(findings []Finding, plans []*Plan) []Finding {
	wanted := map[*Plan]bool{}
	for _, p := range plans {
		wanted[p] = true
	}
	var kept []Finding
	for _, f := range findings {
		if wanted[f.Plan] {
			kept = append(kept, f)
		}
	}
	return kept
}

func Check(c *Corpus) []Finding {
	var findings []Finding
	add := func(p *Plan, rule string, sev Severity, detail string) {
		findings = append(findings, Finding{Plan: p, Rule: rule, Detail: detail, Severity: sev})
	}
	for _, p := range c.Plans {
		switch {
		case !p.HasFront:
			add(p, "no-front-matter", Advisory, "first line is not ---")
		case p.FrontErr != nil:
			add(p, "invalid-front-matter", Failure, strings.Join(strings.Fields(p.FrontErr.Error()), " "))
		default:
			checkFields(c, p, add)
		}
		if p.Directory != "" && !validDirectoryID(p.Front.ID) {
			add(p, "invalid-id", Failure, "directory plan requires ID: twelve digits, a hyphen, and a reference-safe name")
		}
		if matches := c.Lookup(p.Identity); len(matches) > 1 && slices.ContainsFunc(matches, func(match *Plan) bool { return match.Directory != "" }) {
			add(p, "duplicate-id", Failure, fmt.Sprintf("%q identifies %d plans", p.Identity, len(matches)))
		}
		checkLinks(c, p, add)
	}
	checkDuplicateTitles(c, add)
	return SortFindings(findings)
}

// SortFindings orders findings by file, then rule, then detail.
func SortFindings(findings []Finding) []Finding {
	sort.Slice(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.Plan.RelPath != b.Plan.RelPath {
			return a.Plan.RelPath < b.Plan.RelPath
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		return a.Detail < b.Detail
	})
	return findings
}

type addFunc func(p *Plan, rule string, sev Severity, detail string)

func checkFields(c *Corpus, p *Plan, add addFunc) {
	fields := []struct{ name, value string }{
		{"Type", p.Front.Type},
		{"ImplementationStatus", p.Front.ImplementationStatus},
		{"StatusChecked", p.Front.StatusChecked},
		{"StatusNote", p.Front.StatusNote},
	}
	for _, f := range fields {
		if strings.TrimSpace(f.value) == "" {
			add(p, "missing-field", Failure, f.name)
		}
	}
	for _, key := range p.unknownKeys {
		add(p, "unknown-key", Advisory, key)
	}
	if status := p.Front.ImplementationStatus; status != "" && !slices.Contains(StatusValues(), status) {
		add(p, "unknown-status", Failure, fmt.Sprintf("ImplementationStatus %q is not one of %s", status, strings.Join(StatusValues(), ", ")))
	}
	if checked := strings.TrimSpace(p.Front.StatusChecked); checked != "" {
		if _, err := time.Parse("2006-01-02", checked); err != nil {
			add(p, "invalid-date", Failure, fmt.Sprintf("StatusChecked %q is not YYYY-MM-DD", checked))
		}
	}
	checkReference(c, p, "Parent", p.Front.Parent, "parent-not-found", add)
	checkReference(c, p, "SupersededBy", p.Front.SupersededBy, "successor-not-found", add)
	if p.Front.ImplementationStatus == "Superseded" && strings.TrimSpace(p.Front.SupersededBy) == "" {
		add(p, "missing-successor", Advisory, "ImplementationStatus is Superseded but SupersededBy is empty")
	}
	for _, key := range []string{IssuesKey, PullRequestsKey} {
		for _, value := range LinksFor(p.Front, key) {
			if problem := LinkProblem(key, value); problem != "" {
				add(p, "invalid-link", Failure, problem)
			}
		}
	}
}

// checkReference validates a Parent or SupersededBy value: a wikilink must
// match exactly one plan, a path must exist, a URL is taken as is. The cycle
// check applies to parent only, because only parent links form the tree.
func checkReference(c *Corpus, p *Plan, key, value, rule string, add addFunc) {
	if isUndumpedReference(value) {
		add(p, "undumped-reference", Advisory, key+": "+value)
	}
	switch ClassifyParent(value) {
	case ParentNone, ParentURL:
		return
	case ParentWikilink:
		matches := c.Lookup(ReduceLinkTarget(value))
		switch len(matches) {
		case 1:
			if key != "Parent" {
				return
			}
			if chain, cycle := ParentChain(c, p); cycle {
				add(p, "parent-cycle", Failure, strings.Join(chain, " -> "))
			}
		case 0:
			add(p, rule, Failure, fmt.Sprintf("no plan matches %s", value))
		default:
			var paths []string
			for _, m := range matches {
				paths = append(paths, m.RelPath)
			}
			add(p, rule, Failure, fmt.Sprintf("%d plans match %s: %s", len(matches), value, strings.Join(paths, ", ")))
		}
	case ParentPath:
		resolved, err := expandParentPath(value, filepath.Dir(p.Path))
		if err != nil {
			add(p, rule, Failure, fmt.Sprintf("%s: %v", value, err))
			return
		}
		_, err = os.Stat(resolved)
		switch {
		case errors.Is(err, fs.ErrNotExist) && strings.HasPrefix(value, "."):
			add(p, rule, Failure, fmt.Sprintf("%s does not exist (resolved to %s)", value, resolved))
		case errors.Is(err, fs.ErrNotExist):
			add(p, rule, Failure, fmt.Sprintf("%s does not exist", value))
		case err != nil:
			add(p, rule, Failure, fmt.Sprintf("%s: %v", value, err))
		}
	default:
		add(p, rule, Failure, fmt.Sprintf("unrecognised %s form %q (expected \"[[plan]]\", a path, or a URL)", key, value))
	}
}

// checkLinks applies the link rules to the body and to StatusNote. A note
// finding names its source, because the note is not visible in the body.
func checkLinks(c *Corpus, p *Plan, add addFunc) {
	seen := map[string]bool{}
	check := func(link, source string) {
		if seen[link] {
			return
		}
		seen[link] = true
		if isUndumpedReference(link) {
			add(p, "undumped-reference", Advisory, source+link)
		}
		basename := ReduceLinkTarget(link)
		if isPlanStyle(basename) && len(c.Lookup(basename)) == 0 {
			add(p, "dead-link", Failure, source+link)
		}
	}
	for _, link := range p.links {
		check(link, "")
	}
	for _, link := range p.noteLinks {
		check(link, "StatusNote: ")
	}
}

func checkDuplicateTitles(c *Corpus, add addFunc) {
	type key struct{ dir, title string }
	groups := map[key][]*Plan{}
	var order []key
	for _, p := range c.Plans {
		if !p.hasTitle {
			continue
		}
		k := key{dir: filepath.Dir(p.RelPath), title: p.Title}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], p)
	}
	for _, k := range order {
		var group []*Plan
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
			others = append(others, filepath.Base(p.RelPath))
		}
		add(group[0], "duplicate-title", Advisory, fmt.Sprintf("%q is also the title of %s", k.title, strings.Join(others, ", ")))
	}
}

func supersededInTitleGroup(c *Corpus, p *Plan, group []*Plan) bool {
	current := p
	seen := map[*Plan]bool{p: true}
	for current.Valid() && current.Front.ImplementationStatus == "Superseded" {
		var next *Plan
		value := current.Front.SupersededBy
		switch ClassifyParent(value) {
		case ParentWikilink:
			if matches := c.Lookup(ReduceLinkTarget(value)); len(matches) == 1 {
				next = matches[0]
			}
		case ParentPath:
			path, err := expandParentPath(value, filepath.Dir(current.Path))
			if err == nil {
				if resolved, err := filepath.EvalSymlinks(path); err == nil {
					next = c.byPath[resolved]
				}
			}
		}
		if next == nil || !next.Valid() || !slices.Contains(group, next) {
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
