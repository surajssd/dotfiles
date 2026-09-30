package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const statusHelp = `Change a plan's front matter in place. StatusChecked becomes today, the
second argument replaces ImplementationStatus, --note replaces StatusNote,
and --superseded-by sets SupersededBy. Give at least one of the three. Every
other line of the file, including Parent, Type, the link lists, unknown keys,
and the body, is left as it is.

<plan> is a path under the root, a [[wikilink]], a basename, the short name
shown in the NAME column of the tree, or a URL the plan lists. A name must
match exactly one plan.

Recognised ImplementationStatus values:

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
already there. Nothing else in the file changes, StatusChecked included.
Removing a URL is a hand edit that planner check validates.

%s

<plan> is a path under the root, a [[wikilink]], a basename, the short name
shown by planner get, or a URL the plan already lists. On success the
resulting list is printed.`

var linkRules = map[string]string{
	pullRequestsKey: "Every URL must be an http(s) URL, and on github.com it must point at a\npull request (/pull/<n>).",
	issuesKey:       "Every URL must be an http(s) URL: a GitHub issue, a Jira ticket, an Asana\ntask, or any other tracker. A github.com pull request is rejected; it belongs\nunder PullRequests.",
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
			return fmt.Errorf("--superseded-by needs ImplementationStatus Superseded, not %q", target)
		}
		successor, err := canonicalReference(root, opts.supersededBy)
		if err != nil {
			return fmt.Errorf("SupersededBy: %w", err)
		}
		if successor == "[["+p.basename+"]]" {
			return errors.New("a plan cannot supersede itself")
		}
		edits = append(edits, fieldEdit{"SupersededBy", "SupersededBy: " + quoteYAML(successor)})
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
	if err := writePlanFile(p.path, updated); err != nil {
		return err
	}
	reloaded, err := loadPlan(c.root, p.path)
	if err != nil {
		return err
	}
	r := row{node: &node{plan: reloaded}, showRepo: true}
	table := [][]string{listHeader(listOptions{}), r.cells(listOptions{}, deps.now(), "")}
	return writeOutput(deps.stdout, renderTable(table, 0))
}

// runLinks appends URLs to the Issues or PullRequests list of one plan.
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
		if err := writePlanFile(p.path, updated); err != nil {
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

func writePlanFile(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".planner-*.md")
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
	}()
	if err := file.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
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
		edits = append(edits, fieldEdit{"ImplementationStatus", "ImplementationStatus: " + status})
	}
	edits = append(edits, fieldEdit{"StatusChecked", "StatusChecked: " + today})
	if noteSet {
		edits = append(edits, fieldEdit{"StatusNote", "StatusNote: " + quoteYAML(note)})
	}
	return edits
}

func lineEnding(lines []string) string {
	if len(lines) > 0 && strings.HasSuffix(lines[0], "\r\n") {
		return "\r\n"
	}
	return "\n"
}

// YAML key positions keep quoted keys and blank lines inside scalar values
// from being mistaken for field boundaries. Unedited text stays byte-for-byte.
func updateFrontMatter(data []byte, edits []fieldEdit) ([]byte, error) {
	block, _, hasFront, err := splitFrontMatter(string(data))
	if err != nil {
		return nil, err
	}
	if !hasFront {
		return nil, errNoFrontMatter
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(block), &document); err != nil {
		return nil, err
	}
	var fields []*yaml.Node
	indent := ""
	if len(document.Content) > 0 {
		mapping := document.Content[0]
		if mapping.Kind != yaml.MappingNode || mapping.Style&yaml.FlowStyle != 0 {
			return nil, errors.New("front matter must use a block mapping for in-place updates")
		}
		fields = mapping.Content
		if len(fields) > 0 {
			indent = strings.Repeat(" ", fields[0].Column-1)
		}
	}
	lines := strings.SplitAfter(string(data), "\n")
	eol := lineEnding(lines)
	closing := strings.Count(block, "\n") + 1
	byKey := map[string]fieldEdit{}
	for _, edit := range edits {
		byKey[edit.key] = edit
	}
	out := []string{lines[0]}
	seen := map[string]bool{}
	cursor := 1
	for i := 0; i < len(fields); i += 2 {
		key := fields[i]
		edit, ok := byKey[key.Value]
		if !ok {
			continue
		}
		end := closing
		if i+2 < len(fields) {
			end = fields[i+2].Line
		}
		for end > key.Line+1 {
			line := strings.TrimRight(lines[end-1], "\r\n")
			if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, indent+"#") {
				break
			}
			end--
		}
		out = append(out, lines[cursor:key.Line]...)
		out = append(out, renderEdit(edit, eol, indent))
		cursor = end
		seen[key.Value] = true
	}
	out = append(out, lines[cursor:closing]...)
	for _, edit := range edits {
		if !seen[edit.key] {
			out = append(out, renderEdit(edit, eol, indent))
		}
	}
	out = append(out, lines[closing:]...)
	updated := []byte(strings.Join(out, ""))
	if _, err := decodeFrontMatter(updated); err != nil {
		return nil, fmt.Errorf("updated front matter does not decode: %w", err)
	}
	return updated, nil
}

func renderEdit(edit fieldEdit, eol, indent string) string {
	return indent + strings.ReplaceAll(edit.text, "\n", eol+indent) + eol
}

// newFrontMatter puts a fresh block holding Type: plan and the edits in front
// of a file that has none, using the file's own line endings.
func newFrontMatter(data []byte, edits []fieldEdit) []byte {
	eol := lineEnding(strings.SplitAfter(string(data), "\n"))
	lines := []string{"---", "Type: plan"}
	for _, edit := range edits {
		lines = append(lines, edit.text)
	}
	lines = append(lines, "---", "")
	block := strings.Join(lines, eol) + eol
	return append([]byte(block), data...)
}
