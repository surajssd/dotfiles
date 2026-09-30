package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

const (
	prA     = "https://github.com/acme/widgets/pull/1"
	prB     = "https://github.com/acme/widgets/pull/2"
	jiraA   = "https://acme.atlassian.net/browse/WID-7"
	ghIssue = "https://github.com/acme/widgets/issues/3"
)

func TestLinkProblem(t *testing.T) {
	cases := []struct{ key, value, want string }{
		{pullRequestsKey, prA, ""},
		{pullRequestsKey, "https://gitlab.com/acme/widgets/-/merge_requests/4", ""},
		{pullRequestsKey, ghIssue, "is not a GitHub pull request URL"},
		{pullRequestsKey, "https://github.com/acme/widgets", "is not a GitHub pull request URL"},
		{pullRequestsKey, "acme/widgets#1", "is not an http(s) URL"},
		{pullRequestsKey, "", "is not an http(s) URL"},
		{issuesKey, jiraA, ""},
		{issuesKey, ghIssue, ""},
		{issuesKey, "https://app.asana.com/0/123/456", ""},
		{issuesKey, prA, "is a pull request URL"},
		{issuesKey, "ftp://x/y", "is not an http(s) URL"},
	}
	for _, tc := range cases {
		got := linkProblem(tc.key, tc.value)
		if (tc.want == "" && got != "") || (tc.want != "" && !strings.Contains(got, tc.want)) {
			t.Errorf("linkProblem(%s, %q) = %q, want %q", tc.key, tc.value, got, tc.want)
		}
	}
}

func TestUpdateFrontMatterReplacesListsAndFlowSequences(t *testing.T) {
	edit := []fieldEdit{{pullRequestsKey, listLines(pullRequestsKey, []string{"c"})}}
	cases := []struct{ name, in, want string }{
		{"column zero items", "---\npull_requests:\n- a\n- b\nnext: x\n---\n", "---\npull_requests:\n  - c\nnext: x\n---\n"},
		{"flow sequence", "---\npull_requests: [a, b]\nnext: x\n---\n", "---\npull_requests:\n  - c\nnext: x\n---\n"},
		{"indented items", "---\nnext: x\npull_requests:\n  - a\n---\nbody\n", "---\nnext: x\npull_requests:\n  - c\n---\nbody\n"},
		{"missing key", "---\nnext: x\n---\n", "---\nnext: x\npull_requests:\n  - c\n---\n"},
		{"crlf", "---\r\nnext: x\r\n---\r\n", "---\r\nnext: x\r\npull_requests:\r\n  - c\r\n---\r\n"},
	}
	for _, tc := range cases {
		got, err := updateFrontMatter([]byte(tc.in), edit)
		if err != nil || string(got) != tc.want {
			t.Errorf("%s: got %q, %v; want %q", tc.name, got, err, tc.want)
		}
	}
	if _, err := updateFrontMatter([]byte("# no front matter\n"), edit); !errors.Is(err, errNoFrontMatter) {
		t.Errorf("error = %v, want errNoFrontMatter", err)
	}
}

func TestUpdatePRAndIssueAppendAndDedupe(t *testing.T) {
	root, path := statusRoot(t)
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "update", "pr", "--root", root, "widgets-plan", prA, prB, prA); err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(statusFixture, "owner: me\n", "owner: me\npull_requests:\n  - "+prA+"\n  - "+prB+"\n", 1)
	if got := readFile(t, path); got != want {
		t.Errorf("content:\n%s", got)
	}
	if out := io.stdout.String(); out != "Pull Requests:\n  "+prA+"\n  "+prB+"\n" {
		t.Errorf("output:\n%s", out)
	}
	deps, _ = testDependencies("", true, 0)
	if err := run(t, deps, "update", "pr", "--root", root, "widgets-plan", prB); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != want {
		t.Errorf("a repeated URL changed the file:\n%s", got)
	}
	deps, io = testDependencies("", true, 0)
	if err := run(t, deps, "update", "issue", "--root", root, "widgets-plan", jiraA, ghIssue); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, path)
	if !strings.Contains(got, "pull_requests:\n  - "+prA+"\n  - "+prB+"\nissues:\n  - "+jiraA+"\n  - "+ghIssue+"\n---\n") {
		t.Errorf("content:\n%s", got)
	}
	if !strings.Contains(got, "status_checked: 2026-09-01\n") {
		t.Error("status_checked changed")
	}
	if !strings.HasPrefix(io.stdout.String(), "Issues:\n  "+jiraA+"\n") {
		t.Errorf("output:\n%s", io.stdout)
	}
}

func TestUpdateLinksRejectsBadInput(t *testing.T) {
	root, path := statusRoot(t)
	writeFile(t, filepath.Join(root, "github.com", "acme", "widgets", "260911120000-legacy.md"), "# Legacy\n")
	cases := map[string][]string{
		"is not an http(s) URL":        {"pr", "widgets-plan", "acme/widgets#1"},
		"is not a GitHub pull request": {"pr", "widgets-plan", ghIssue},
		"is a pull request URL":        {"issue", "widgets-plan", prA},
		"has no front matter":          {"pr", "legacy", prA},
		"plan not found":               {"pr", "nope", prA},
	}
	for want, args := range cases {
		deps, _ := testDependencies("", true, 0)
		err := run(t, deps, append([]string{"update", args[0], "--root", root}, args[1:]...)...)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%v: error = %v, want %q", args, err, want)
		}
	}
	if readFile(t, path) != statusFixture {
		t.Error("a rejected command changed the file")
	}
}

func TestUpdateStatusSupersededBy(t *testing.T) {
	root, path := statusRoot(t)
	writeFile(t, filepath.Join(root, "github.com", "acme", "widgets", "260920120000-successor.md"), "---\ntype: plan\nimplementation_status: InProgress\nstatus_checked: 2026-09-01\nstatus_note: \"New.\"\n---\n# Successor\n")
	deps, _ := testDependencies("", true, 0)
	err := run(t, deps, "update", "status", "--root", root, "widgets-plan", "--superseded-by", "successor")
	if err == nil || !strings.Contains(err.Error(), "Superseded") {
		t.Fatalf("InProgress plan accepted a successor: %v", err)
	}
	err = run(t, deps, "update", "status", "--root", root, "widgets-plan", "Superseded", "--superseded-by", "widgets-plan")
	if err == nil || !strings.Contains(err.Error(), "itself") {
		t.Fatalf("self reference: %v", err)
	}
	if readFile(t, path) != statusFixture {
		t.Fatal("a rejected command changed the file")
	}
	if err := run(t, deps, "update", "status", "--root", root, "widgets-plan", "Superseded", "--note", "Folded.", "--superseded-by", "successor"); err != nil {
		t.Fatal(err)
	}
	want := "---\ntype: plan\nparent: \"[[260901000000-parent-plan]]\"\nimplementation_status: Superseded\nstatus_checked: 2026-09-29\nstatus_note: \"Folded.\"\nowner: me\nsuperseded_by: \"[[260920120000-successor]]\"\n---\n\n# Title\n\nBody with status_checked: text that must not change.\n"
	if got := readFile(t, path); got != want {
		t.Errorf("content:\n%s", got)
	}
	if err := run(t, deps, "update", "status", "--root", root, "widgets-plan", "--superseded-by", "https://example.com/replacement"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); !strings.Contains(got, "\nsuperseded_by: \"https://example.com/replacement\"\n---\n") {
		t.Errorf("content:\n%s", got)
	}
}

func TestPlansAreFoundByURL(t *testing.T) {
	fixtureHome(t)
	root := fixtureRoot(t)
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "get", "--root", root, "https://github.com/acme/gadgets/pull/3"); err != nil {
		t.Fatal(err)
	}
	if out := io.stdout.String(); !strings.Contains(out, "widgets-links-fields") || strings.Count(out, "\n") != 2 {
		t.Errorf("output:\n%s", out)
	}
	deps, io = testDependencies("", true, 0)
	if err := run(t, deps, "describe", "--root", root, "https://github.com/acme/widgets/issues/7"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(io.stdout.String(), "widgets-url-parent") {
		t.Errorf("a URL parent was not matched:\n%s", io.stdout)
	}
	deps, _ = testDependencies("", true, 0)
	err := run(t, deps, "update", "pr", "--root", root, "https://acme.atlassian.net/browse/WID-1", prA)
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("two plans list WID-1: error = %v", err)
	}
	err = run(t, deps, "get", "--root", root, "https://example.com/nothing")
	if err == nil || !strings.Contains(err.Error(), "links to") {
		t.Errorf("error = %v", err)
	}
}

func TestNewIssueAndPRFlags(t *testing.T) {
	initRepo(t, "upstream", "https://github.com/acme/widgets")
	root := t.TempDir()
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "new", "--root", root, "--issue", jiraA, "--pr", prA, "--pr", prB, "--pr", prA, "linked"); err != nil {
		t.Fatal(err)
	}
	want := "---\ntype: plan\nissues:\n  - " + jiraA + "\npull_requests:\n  - " + prA + "\n  - " + prB + "\nimplementation_status: NotImplemented\nstatus_checked: 2026-09-29\nstatus_note: \"Implementation has not started.\"\n---\n\n"
	if got := readFile(t, strings.TrimSpace(io.stdout.String())); got != want {
		t.Errorf("content:\n%s", got)
	}

	piped := "---\ntype: plan\npull_requests:\n  - " + prA + "\nimplementation_status: InProgress\nstatus_checked: 2026-09-01\nstatus_note: \"Mine.\"\n---\n\n# Piped\n"
	deps, io = testDependencies(piped, false, 0)
	if err := run(t, deps, "new", "--root", root, "--pr", prB, "--issue", ghIssue, "piped links"); err != nil {
		t.Fatal(err)
	}
	want = "---\ntype: plan\npull_requests:\n  - " + prA + "\n  - " + prB + "\nimplementation_status: InProgress\nstatus_checked: 2026-09-01\nstatus_note: \"Mine.\"\nissues:\n  - " + ghIssue + "\n---\n\n# Piped\n"
	if got := readFile(t, strings.TrimSpace(io.stdout.String())); got != want {
		t.Errorf("content:\n%s", got)
	}

	before := listFiles(t, root)
	deps, _ = testDependencies("", true, 0)
	err := run(t, deps, "new", "--root", root, "--pr", ghIssue, "rejected")
	if err == nil || !strings.Contains(err.Error(), "pull request") {
		t.Fatalf("error = %v", err)
	}
	if after := listFiles(t, root); len(after) != len(before) {
		t.Errorf("a rejected command created a file: %v", after)
	}
}
