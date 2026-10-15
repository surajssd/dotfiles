package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	prOne   = "https://github.com/acme/widgets/pull/1"
	prTwo   = "https://github.com/acme/widgets/pull/2"
	prThree = "https://github.com/acme/gadgets/pull/3"
	prFour  = "https://github.com/acme/widgets/pull/4"
)

// fakeGhPR puts a gh on PATH that answers gh pr view <url> --json state from
// the table and fails, gh style, for any other URL.
func fakeGhPR(t *testing.T, states map[string]string) {
	t.Helper()
	bin := t.TempDir()
	var script strings.Builder
	script.WriteString("#!/bin/sh\n[ \"$1 $2\" = \"pr view\" ] || exit 2\ncase \"$3\" in\n")
	for url, state := range states {
		fmt.Fprintf(&script, "  %q) printf '{\"state\":\"%s\"}\\n' ;;\n", url, state)
	}
	script.WriteString("  *) echo 'GraphQL: Could not resolve to a PullRequest with the number of 4.' >&2; echo 'second line' >&2; exit 1 ;;\nesac\n")
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script.String()), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// githubRoot holds one active plan whose note calls #3 open although it is
// closed, mentions #1 only as "Opened", and names #4, which gh cannot answer.
func githubRoot(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "github.com", "acme", "widgets", "260910120000-widgets-plan.md")
	note := "PR #1 merged as abc123; Opened #1 on Monday. PR #2 and #3 are open and need review! Reopen #4 if the open question returns."
	writeFile(t, path, "---\nType: plan\nImplementationStatus: InProgress\nStatusChecked: 2026-09-29\nStatusNote: \""+note+"\"\nPullRequests:\n  - "+prOne+"\n  - "+prTwo+"\n  - "+prThree+"\n  - "+prFour+"\n---\n# Widgets plan\n")
	writeFile(t, filepath.Join(root, "github.com", "acme", "widgets", "260911120000-widgets-done.md"), "---\nType: plan\nImplementationStatus: Implemented\nStatusChecked: 2026-09-29\nStatusNote: \"#1 is open.\"\nPullRequests:\n  - "+prOne+"\n---\n# Widgets done\n")
	fakeGhPR(t, map[string]string{prOne: "MERGED", prTwo: "OPEN", prThree: "CLOSED"})
	return root, path
}

func TestCheckGitHubFlagsStaleNotesAndUnknownStates(t *testing.T) {
	root, _ := githubRoot(t)
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "check", "--root", root, "--github"); err != nil {
		t.Fatal(err)
	}
	out := io.stdout.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("want a header and two rows:\n%s", out)
	}
	if !strings.Contains(lines[1], "pr-state-unknown") || !strings.Contains(lines[1], prFour+": GraphQL: Could not resolve to a PullRequest with the number of 4.") || strings.Contains(lines[1], "second line") {
		t.Errorf("unknown state row: %s", lines[1])
	}
	if !strings.Contains(lines[2], "stale-pr-note") || !strings.Contains(lines[2], "StatusNote calls #3 open; GitHub says CLOSED") {
		t.Errorf("stale note row: %s", lines[2])
	}
	if strings.Contains(out, "widgets-done") {
		t.Errorf("an Implemented plan was checked against GitHub:\n%s", out)
	}
	deps, io = testDependencies("", true, 0)
	if err := run(t, deps, "check", "--root", root, "--github", "widgets-done"); err != nil {
		t.Fatal(err)
	}
	if out := io.stdout.String(); !strings.Contains(out, "widgets-done") || !strings.Contains(out, "StatusNote calls #1 open; GitHub says MERGED") {
		t.Errorf("a named inactive plan was not checked:\n%s", out)
	}
	deps, io = testDependencies("", true, 0)
	if err := run(t, deps, "check", "--root", root); err != nil || io.stdout.Len() != 0 || io.stderr.String() != "No findings.\n" {
		t.Errorf("without --github: error = %v, stdout = %q, stderr = %q", err, io.stdout, io.stderr)
	}
}

func TestDescribeGitHubShowsStates(t *testing.T) {
	root, _ := githubRoot(t)
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "describe", "--root", root, "--github", "widgets-plan"); err != nil {
		t.Fatal(err)
	}
	out := io.stdout.String()
	for _, want := range []string{
		"  " + prOne + "  MERGED\n",
		"  " + prTwo + "  OPEN\n",
		"  " + prThree + "  CLOSED\n",
		"  " + prFour + "  state unknown: GraphQL: Could not resolve to a PullRequest with the number of 4.\n",
		"advisory  stale-pr-note         StatusNote calls #3 open; GitHub says CLOSED",
		"advisory  pr-state-unknown      " + prFour,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("describe --github lacks %q:\n%s", want, out)
		}
	}
	deps, io = testDependencies("", true, 0)
	if err := run(t, deps, "describe", "--root", root, "widgets-plan"); err != nil {
		t.Fatal(err)
	}
	if out := io.stdout.String(); !strings.Contains(out, "  "+prOne+"\n") || strings.Contains(out, "MERGED") || strings.Contains(out, "stale-pr-note") {
		t.Errorf("describe without --github touched GitHub:\n%s", out)
	}
}

func TestGitHubFindingsNumberRules(t *testing.T) {
	p := &plan{hasFront: true, front: frontMatter{
		ImplementationStatus: "InProgress",
		PullRequests:         []string{prOne, "https://github.com/acme/gadgets/pull/1", prTwo, "https://gitlab.com/acme/widgets/-/merge_requests/9"},
		StatusNote:           "#1 is open. #12 is open. https://gitlab.com/acme/widgets/-/merge_requests/9 stays open. " + prTwo + " is open.",
	}}
	states := map[string]prState{
		prOne:                                    {state: "MERGED"},
		"https://github.com/acme/gadgets/pull/1": {state: "OPEN"},
		prTwo:                                    {state: "MERGED"},
		"https://gitlab.com/acme/widgets/-/merge_requests/9": {state: "MERGED"},
	}
	var details []string
	for _, f := range githubFindings([]*plan{p}, states) {
		details = append(details, f.detail)
	}
	want := []string{
		"StatusNote calls #2 open; GitHub says MERGED",
		"StatusNote calls https://gitlab.com/acme/widgets/-/merge_requests/9 open; GitHub says MERGED",
	}
	if strings.Join(details, "\n") != strings.Join(want, "\n") {
		t.Errorf("details = %q, want %q", details, want)
	}
}
