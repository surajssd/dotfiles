package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const statusHelp = `Change a plan's front matter in place. status_checked becomes today, the
second argument replaces implementation_status, and --note replaces
status_note. Give at least one of the two. Every other line of the file,
including parent, type, unknown keys, and the body, is left as it is.

<plan> is a path under the root, a [[wikilink]], a basename, or the short name
shown in the NAME column of the tree. A name must match exactly one plan.

Recognised implementation_status values:

  NotImplemented, InProgress, PartiallyImplemented, ImplementedUnmerged,
  Implemented, Superseded

The status may be typed in any case, with or without hyphens: in-progress and
inprogress both mean InProgress.

A plan without front matter receives a new block when both a status and --note
are given. On success the plan's tree row is printed.`

func runStatus(deps dependencies, root string, args []string, note string, noteSet bool) error {
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
	if status == "" && !noteSet {
		return errors.New("nothing to change: give a new status, --note, or both")
	}
	if p.hasFront && p.frontErr != nil {
		return fmt.Errorf("%s: front matter does not decode (%v); fix it before changing its status", p.relPath, p.frontErr)
	}
	data, err := os.ReadFile(p.path)
	if err != nil {
		return err
	}
	updated, err := updateFrontMatter(data, status, note, noteSet, deps.now().Format(dateLayout))
	if err != nil {
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

// updateFrontMatter rewrites the three status lines of a front matter block
// and keeps every other byte. Missing keys are appended before the closing
// delimiter; a replaced key also drops the indented continuation lines of a
// block scalar. A file without front matter gets a fresh block.
func updateFrontMatter(data []byte, status, note string, noteSet bool, today string) ([]byte, error) {
	lines := strings.SplitAfter(string(data), "\n")
	eol := "\n"
	if len(lines) > 0 && strings.HasSuffix(lines[0], "\r\n") {
		eol = "\r\n"
	}
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r\n") != "---" {
		if status == "" || !noteSet {
			return nil, errors.New("the plan has no front matter; give both a status and --note to add one")
		}
		block := strings.Join([]string{
			"---",
			"type: plan",
			"implementation_status: " + status,
			"status_checked: " + today,
			"status_note: " + quoteYAML(note),
			"---",
			"",
		}, eol) + eol
		return append([]byte(block), data...), nil
	}
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
	replacements := map[string]string{"status_checked": "status_checked: " + today}
	if status != "" {
		replacements["implementation_status"] = "implementation_status: " + status
	}
	if noteSet {
		replacements["status_note"] = "status_note: " + quoteYAML(note)
	}
	out := []string{lines[0]}
	seen := map[string]bool{}
	skipContinuation := false
	for _, line := range lines[1:closing] {
		indented := strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
		if skipContinuation && indented {
			continue
		}
		skipContinuation = false
		key, _, found := strings.Cut(line, ":")
		key = strings.TrimSpace(key)
		if replacement, ok := replacements[key]; found && ok && !indented && !seen[key] {
			out = append(out, replacement+eol)
			seen[key] = true
			skipContinuation = true
			continue
		}
		out = append(out, line)
	}
	for _, key := range []string{"implementation_status", "status_checked", "status_note"} {
		if replacement, ok := replacements[key]; ok && !seen[key] {
			out = append(out, replacement+eol)
		}
	}
	out = append(out, lines[closing:]...)
	return []byte(strings.Join(out, "")), nil
}
