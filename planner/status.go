package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const statusHelp = `Change a plan's front matter in place. status_checked becomes today, the
second argument replaces implementation_status, --note replaces status_note,
and --superseded-by sets superseded_by. Give at least one of the three. Every
other line of the file, including parent, type, the link lists, unknown keys,
and the body, is left as it is.

<plan> is a path under the root, a [[wikilink]], a basename, the short name
shown in the NAME column of the tree, or a URL the plan lists. A name must
match exactly one plan.

Recognised implementation_status values:

  NotImplemented, InProgress, PartiallyImplemented, ImplementedUnmerged,
  Implemented, Superseded

The status may be typed in any case, with or without hyphens: in-progress and
inprogress both mean InProgress.

--superseded-by takes the same forms as --parent on planner create (a dumped
plan, a file path, or a URL) and needs the status, new or current, to be
Superseded.

A plan without front matter receives a new block when both a status and --note
are given. On success the plan's table row is printed.`

const linksHelp = `Add URLs to the %s list of a plan's front matter, skipping any that are
already there. Nothing else in the file changes, status_checked included.
Removing a URL is a hand edit that planner check validates.

%s

<plan> is a path under the root, a [[wikilink]], a basename, the short name
shown by planner get, or a URL the plan already lists. On success the
resulting list is printed.`

var linkRules = map[string]string{
	pullRequestsKey: "Every URL must be an http(s) URL, and on github.com it must point at a\npull request (/pull/<n>).",
	issuesKey:       "Every URL must be an http(s) URL: a GitHub issue, a Jira ticket, an Asana\ntask, or any other tracker. A github.com pull request is rejected; it belongs\nunder pull_requests.",
}

type statusOptions struct {
	note         string
	noteSet      bool
	supersededBy string
}

func runStatus(deps dependencies, root string, args []string, opts statusOptions) error {
	c, err := loadCorpus(root)
	if err != nil {
		return err
	}
	p, err := findPlan(c, args[0])
	if err != nil {
		return err
	}
	status := ""
	if len(args) == 2 {
		var ok bool
		if status, ok = canonicalStatus(args[1]); !ok {
			return fmt.Errorf("unknown status %q; use one of %s", args[1], strings.Join(statusValues, ", "))
		}
	}
	if status == "" && !opts.noteSet && opts.supersededBy == "" {
		return errors.New("nothing to change: give a new status, --note, --superseded-by, or several of them")
	}
	if p.hasFront && p.frontErr != nil {
		return fmt.Errorf("%s: front matter does not decode (%v); fix it before changing its status", p.relPath, p.frontErr)
	}
	edits := statusEdits(status, opts.note, opts.noteSet, deps.now().Format(dateLayout))
	if opts.supersededBy != "" {
		target := status
		if target == "" {
			target = p.front.ImplementationStatus
		}
		if target != "Superseded" {
			return fmt.Errorf("--superseded-by needs implementation_status Superseded, not %q", target)
		}
		successor, err := canonicalReference(root, opts.supersededBy)
		if err != nil {
			return fmt.Errorf("superseded_by: %w", err)
		}
		if successor == "[["+p.basename+"]]" {
			return errors.New("a plan cannot supersede itself")
		}
		edits = append(edits, fieldEdit{"superseded_by", "superseded_by: " + quoteYAML(successor)})
	}
	data, err := os.ReadFile(p.path)
	if err != nil {
		return err
	}
	updated, err := updateFrontMatter(data, edits)
	switch {
	case errors.Is(err, errNoFrontMatter) && (status == "" || !opts.noteSet):
		return fmt.Errorf("%s: the plan has no front matter; give both a status and --note to add one", p.relPath)
	case errors.Is(err, errNoFrontMatter):
		updated = newFrontMatter(data, edits)
	case err != nil:
		return fmt.Errorf("%s: %w", p.relPath, err)
	}
	if err := os.WriteFile(p.path, updated, 0o644); err != nil {
		return err
	}
	reloaded, err := loadPlan(root, p.path)
	if err != nil {
		return err
	}
	r := row{node: &node{plan: reloaded}, showRepo: true}
	table := [][]string{listHeader(listOptions{}), r.cells(listOptions{}, deps.now(), "")}
	return writeOutput(deps.stdout, renderTable(table, 0))
}

// runLinks appends URLs to the issues or pull_requests list of one plan.
func runLinks(deps dependencies, root, key string, args []string) error {
	for _, value := range args[1:] {
		if problem := linkProblem(key, value); problem != "" {
			return errors.New(problem)
		}
	}
	c, err := loadCorpus(root)
	if err != nil {
		return err
	}
	p, err := findPlan(c, args[0])
	if err != nil {
		return err
	}
	switch {
	case !p.hasFront:
		return fmt.Errorf("%s: the plan has no front matter; give it a status and --note with planner set status first", p.relPath)
	case p.frontErr != nil:
		return fmt.Errorf("%s: front matter does not decode (%v); fix it before adding links", p.relPath, p.frontErr)
	}
	existing := mergeLinks(linksFor(p.front, key), nil)
	merged := mergeLinks(existing, args[1:])
	if len(merged) > len(existing) {
		data, err := os.ReadFile(p.path)
		if err != nil {
			return err
		}
		updated, err := updateFrontMatter(data, []fieldEdit{{key, listLines(key, merged)}})
		if err != nil {
			return fmt.Errorf("%s: %w", p.relPath, err)
		}
		if err := os.WriteFile(p.path, updated, 0o644); err != nil {
			return err
		}
	}
	var out strings.Builder
	out.WriteString(linkLabel(key) + ":\n")
	for _, value := range merged {
		out.WriteString("  " + value + "\n")
	}
	return writeOutput(deps.stdout, out.String())
}

// fieldEdit is one front matter key with its rendered lines, without the
// trailing newline; a list value spans several lines.
type fieldEdit struct {
	key  string
	text string
}

var errNoFrontMatter = errors.New("the plan has no front matter")

// statusEdits renders the status lines a status change touches.
func statusEdits(status, note string, noteSet bool, today string) []fieldEdit {
	var edits []fieldEdit
	if status != "" {
		edits = append(edits, fieldEdit{"implementation_status", "implementation_status: " + status})
	}
	edits = append(edits, fieldEdit{"status_checked", "status_checked: " + today})
	if noteSet {
		edits = append(edits, fieldEdit{"status_note", "status_note: " + quoteYAML(note)})
	}
	return edits
}

func lineEnding(lines []string) string {
	if len(lines) > 0 && strings.HasSuffix(lines[0], "\r\n") {
		return "\r\n"
	}
	return "\n"
}

// updateFrontMatter rewrites the edited keys of a front matter block and keeps
// every other byte. A replaced key also drops its continuation lines: the
// indented lines of a block scalar or list, and list items written at column
// zero. Missing keys are appended before the closing delimiter in the order
// given. A file without front matter yields errNoFrontMatter.
func updateFrontMatter(data []byte, edits []fieldEdit) ([]byte, error) {
	lines := strings.SplitAfter(string(data), "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r\n") != "---" {
		return nil, errNoFrontMatter
	}
	eol := lineEnding(lines)
	closing := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r\n") == "---" {
			closing = i
			break
		}
	}
	if closing < 0 {
		return nil, errors.New("front matter has no closing ---")
	}
	byKey := map[string]fieldEdit{}
	for _, edit := range edits {
		byKey[edit.key] = edit
	}
	out := []string{lines[0]}
	seen := map[string]bool{}
	skipContinuation := false
	for _, line := range lines[1:closing] {
		continuation := strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "- ")
		if skipContinuation && continuation {
			continue
		}
		skipContinuation = false
		key, _, found := strings.Cut(line, ":")
		key = strings.TrimSpace(key)
		if edit, ok := byKey[key]; found && ok && !continuation && !seen[key] {
			out = append(out, renderEdit(edit, eol))
			seen[key] = true
			skipContinuation = true
			continue
		}
		out = append(out, line)
	}
	for _, edit := range edits {
		if !seen[edit.key] {
			out = append(out, renderEdit(edit, eol))
		}
	}
	out = append(out, lines[closing:]...)
	return []byte(strings.Join(out, "")), nil
}

func renderEdit(edit fieldEdit, eol string) string {
	return strings.ReplaceAll(edit.text, "\n", eol) + eol
}

// newFrontMatter puts a fresh block holding type: plan and the edits in front
// of a file that has none, using the file's own line endings.
func newFrontMatter(data []byte, edits []fieldEdit) []byte {
	eol := lineEnding(strings.SplitAfter(string(data), "\n"))
	lines := []string{"---", "type: plan"}
	for _, edit := range edits {
		lines = append(lines, edit.text)
	}
	lines = append(lines, "---", "")
	block := strings.Join(lines, eol) + eol
	return append([]byte(block), data...)
}
