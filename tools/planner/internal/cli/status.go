package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/surajssd/dotfiles/tools/planner/internal/plan"
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
  Implemented, Superseded, Abandoned

The status may be typed in any case, with or without hyphens: in-progress and
inprogress both mean InProgress.

Abandoned means the plan will not be implemented. Like Implemented and
Superseded, it is inactive and hidden by default.

--note-file reads the note from a file instead, or from stdin when the path
is -, dropping blank lines at either end. It cannot be combined with --note.

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

const parentHelp = `Set the Parent of a plan's front matter, replacing the value it already
has. Nothing else in the file changes, StatusChecked included. Clearing a
parent is a hand edit that planner check validates.

<ref> takes the forms --parent accepts on planner create: a dumped plan (as
[[wikilink]], basename, or the short name shown by planner get), a file path,
or a URL. A plan under the root is stored as the wikilink of its effective identity, a
file outside the root as a path, and a URL as is. A plan cannot be its own
parent, and the new parent cannot be one of the plan's descendants.

<plan> is a path under the root, a [[wikilink]], a basename, the short name
shown by planner get, or a URL the plan lists. On success the stored Parent
line is printed.`

var linkRules = map[string]string{
	plan.PullRequestsKey: "Every URL must be an http(s) URL, and on github.com it must point at a\npull request (/pull/<n>).",
	plan.IssuesKey:       "Every URL must be an http(s) URL: a GitHub issue, a Jira ticket, an Asana\ntask, or any other tracker. A github.com pull request is rejected; it belongs\nunder PullRequests.",
}

type statusOptions struct {
	note         string
	noteSet      bool
	noteFile     string
	supersededBy string
}

func runStatus(deps dependencies, root string, args []string, opts statusOptions) error {
	if opts.noteFile != "" {
		if opts.noteSet {
			return errors.New("give --note or --note-file, not both")
		}
		note, err := readNoteFile(deps, opts.noteFile)
		if err != nil {
			return err
		}
		opts.note, opts.noteSet = note, true
	}
	c, err := plan.Load(root)
	if err != nil {
		return err
	}
	p, err := plan.Find(c, args[0])
	if err != nil {
		return err
	}
	status := ""
	if len(args) == 2 {
		var ok bool
		if status, ok = plan.CanonicalStatus(args[1]); !ok {
			return fmt.Errorf("unknown status %q; use one of %s", args[1], strings.Join(plan.StatusValues(), ", "))
		}
	}
	if status == "" && !opts.noteSet && opts.supersededBy == "" {
		return errors.New("nothing to change: give a new status, --note, --superseded-by, or several of them")
	}
	if p.HasFront && p.FrontErr != nil {
		return fmt.Errorf("%s: front matter does not decode (%v); fix it before changing its status", p.RelPath, p.FrontErr)
	}
	change := plan.Change{
		Status:  status,
		Checked: deps.now().Format(plan.DateLayout),
		Note:    opts.note,
		NoteSet: opts.noteSet,
	}
	if opts.supersededBy != "" {
		target := status
		if target == "" {
			target = p.Front.ImplementationStatus
		}
		if target != "Superseded" {
			return fmt.Errorf("--superseded-by needs ImplementationStatus Superseded, not %q", target)
		}
		successor, err := plan.CanonicalReference(root, opts.supersededBy)
		if err != nil {
			return fmt.Errorf("SupersededBy: %w", err)
		}
		if successor == "[["+p.Identity+"]]" {
			return errors.New("a plan cannot supersede itself")
		}
		change.SupersededBy = successor
	}
	if !p.HasFront && (status == "" || !opts.noteSet) {
		return fmt.Errorf("%s: the plan has no front matter; give both a status and --note to add one", p.RelPath)
	}
	if _, err := plan.EditFile(p, change); err != nil {
		return err
	}
	reloaded, err := plan.LoadFile(c.Root, p.Path)
	if err != nil {
		return err
	}
	r := row{node: &node{plan: reloaded}, showRepo: true}
	table := [][]string{listHeader(listOptions{}), r.cells(listOptions{}, deps.now(), "")}
	return writeOutput(deps.stdout, renderTable(table, 0))
}

// readNoteFile returns the text of a --note-file, or of stdin for -, without
// the blank lines at either end.
func readNoteFile(deps dependencies, path string) (string, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(deps.stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return "", fmt.Errorf("--note-file: %w", err)
	}
	note := strings.Trim(string(data), "\r\n")
	if strings.TrimSpace(note) == "" {
		return "", fmt.Errorf("--note-file %s holds no text", path)
	}
	return note, nil
}

// runLinks appends URLs to the Issues or PullRequests list of one plan.
func runLinks(deps dependencies, root, key string, args []string) error {
	for _, value := range args[1:] {
		if problem := plan.LinkProblem(key, value); problem != "" {
			return errors.New(problem)
		}
	}
	c, err := plan.Load(root)
	if err != nil {
		return err
	}
	p, err := plan.Find(c, args[0])
	if err != nil {
		return err
	}
	switch {
	case !p.HasFront:
		return fmt.Errorf("%s: the plan has no front matter; give it a status and --note with planner set status first", p.RelPath)
	case p.FrontErr != nil:
		return fmt.Errorf("%s: front matter does not decode (%v); fix it before adding links", p.RelPath, p.FrontErr)
	}
	change := plan.Change{}
	if key == plan.PullRequestsKey {
		change.PullRequests = args[1:]
	} else {
		change.Issues = args[1:]
	}
	front, err := plan.EditFile(p, change)
	if err != nil {
		return err
	}
	var out strings.Builder
	out.WriteString(linkLabel(key) + ":\n")
	for _, value := range plan.LinksFor(front, key) {
		out.WriteString("  " + value + "\n")
	}
	return writeOutput(deps.stdout, out.String())
}

// runSetParent replaces the Parent of one plan.
func runSetParent(deps dependencies, root string, args []string) error {
	c, err := plan.Load(root)
	if err != nil {
		return err
	}
	p, err := plan.Find(c, args[0])
	if err != nil {
		return err
	}
	switch {
	case !p.HasFront:
		return fmt.Errorf("%s: the plan has no front matter; give it a status and --note with planner set status first", p.RelPath)
	case p.FrontErr != nil:
		return fmt.Errorf("%s: front matter does not decode (%v); fix it before setting its parent", p.RelPath, p.FrontErr)
	}
	value, err := plan.CanonicalReference(root, args[1])
	if err != nil {
		return err
	}
	if value == "[["+p.Identity+"]]" {
		return errors.New("a plan cannot be its own parent")
	}
	if chain := plan.ParentCycle(c, p, value); chain != nil {
		return fmt.Errorf("parent would form a cycle: %s", strings.Join(chain, " -> "))
	}
	if _, err := plan.EditFile(p, plan.Change{Parent: value}); err != nil {
		return err
	}
	return writeOutput(deps.stdout, "Parent: "+value+"\n")
}
