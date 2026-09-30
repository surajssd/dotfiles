package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const statusFixture = "---\ntype: plan\nparent: \"[[260901000000-parent-plan]]\"\nimplementation_status: InProgress\nstatus_checked: 2026-09-01\nstatus_note: \"Old note.\"\nowner: me\n---\n\n# Title\n\nBody with status_checked: text that must not change.\n"

func statusRoot(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "github.com", "acme", "widgets", "260910120000-widgets-plan.md")
	writeFile(t, path, statusFixture)
	return root, path
}

func TestStatusChangesStatusDateAndNoteOnly(t *testing.T) {
	root, path := statusRoot(t)
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "set", "status", "--root", root, "widgets-plan", "Implemented", "--note", `Merged in "PR #7".`); err != nil {
		t.Fatal(err)
	}
	want := "---\ntype: plan\nparent: \"[[260901000000-parent-plan]]\"\nimplementation_status: Implemented\nstatus_checked: 2026-09-29\nstatus_note: \"Merged in \\\"PR #7\\\".\"\nowner: me\n---\n\n# Title\n\nBody with status_checked: text that must not change.\n"
	if got := readFile(t, path); got != want {
		t.Errorf("content:\n%s", got)
	}
	out := io.stdout.String()
	if !strings.HasPrefix(out, "NAME") || !strings.Contains(out, "widgets-plan") || !strings.Contains(out, "Implemented") || !strings.Contains(out, "0d") {
		t.Errorf("output:\n%s", out)
	}
}

func TestStatusReferenceForms(t *testing.T) {
	root, path := statusRoot(t)
	t.Chdir(filepath.Dir(path))
	for i, ref := range []string{"widgets-plan", "260910120000-widgets-plan", "260910120000-widgets-plan.md", "[[260910120000-widgets-plan|alias]]", path, "./260910120000-widgets-plan.md"} {
		spelling := []string{"InProgress", "in-progress", "inprogress", "IN-PROGRESS"}[i%4]
		deps, _ := testDependencies("", true, 0)
		if err := run(t, deps, "set", "status", "--root", root, ref, spelling); err != nil {
			t.Errorf("%s: %v", ref, err)
		}
	}
	if !strings.Contains(readFile(t, path), "implementation_status: InProgress\nstatus_checked: 2026-09-29\nstatus_note: \"Old note.\"") {
		t.Errorf("touch changed more than the date:\n%s", readFile(t, path))
	}
}

func TestStatusNoteOnly(t *testing.T) {
	root, path := statusRoot(t)
	deps, _ := testDependencies("", true, 0)
	if err := run(t, deps, "set", "status", "--root", root, "widgets-plan", "--note", "Still going."); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); !strings.Contains(got, "implementation_status: InProgress\nstatus_checked: 2026-09-29\nstatus_note: \"Still going.\"\n") {
		t.Errorf("content:\n%s", got)
	}
}

func TestStatusRejectsBadInput(t *testing.T) {
	root, path := statusRoot(t)
	writeFile(t, filepath.Join(root, "github.com", "acme", "gadgets", "260911120000-widgets-plan.md"), statusFixture)
	cases := map[string][]string{
		"nothing to change": {"260910120000-widgets-plan"},
		"unknown status":    {"260910120000-widgets-plan", "done"},
		"not found":         {"nope", "Implemented"},
		"ambiguous":         {"widgets-plan", "Implemented"},
		"is not a plan":     {"/no/such/plan.md", "Implemented"},
	}
	for want, args := range cases {
		deps, _ := testDependencies("", true, 0)
		err := run(t, deps, append([]string{"set", "status", "--root", root}, args...)...)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%v: error = %v, want %q", args, err, want)
		}
	}
	if readFile(t, path) != statusFixture {
		t.Error("a rejected command changed the file")
	}
}

func TestStatusAddsFrontMatterToLegacyPlan(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "github.com", "acme", "widgets", "260912120000-legacy.md")
	writeFile(t, path, "# Legacy\r\n\r\nBody.\r\n")
	deps, _ := testDependencies("", true, 0)
	if err := run(t, deps, "set", "status", "--root", root, "legacy", "Implemented"); err == nil || !strings.Contains(err.Error(), "--note") {
		t.Fatalf("error = %v", err)
	}
	if err := run(t, deps, "set", "status", "--root", root, "legacy", "Implemented", "--note", "Shipped long ago."); err != nil {
		t.Fatal(err)
	}
	want := "---\r\ntype: plan\r\nimplementation_status: Implemented\r\nstatus_checked: 2026-09-29\r\nstatus_note: \"Shipped long ago.\"\r\n---\r\n\r\n# Legacy\r\n\r\nBody.\r\n"
	if got := readFile(t, path); got != want {
		t.Errorf("content:\n%q", got)
	}
}

func TestStatusReplacesBlockScalarNoteAndAppendsMissingKeys(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "github.com", "acme", "widgets", "260913120000-folded.md")
	writeFile(t, path, "---\ntype: plan\nstatus_note: >\n  first line\n  second line\nimplementation_status: InProgress\n---\n# Folded\n")
	deps, _ := testDependencies("", true, 0)
	if err := run(t, deps, "set", "status", "--root", root, "folded", "Superseded", "--note", "Replaced."); err != nil {
		t.Fatal(err)
	}
	want := "---\ntype: plan\nstatus_note: \"Replaced.\"\nimplementation_status: Superseded\nstatus_checked: 2026-09-29\n---\n# Folded\n"
	if got := readFile(t, path); got != want {
		t.Errorf("content:\n%s", got)
	}
}

func TestStatusRefusesBrokenFrontMatter(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "github.com", "acme", "widgets", "260914120000-broken.md")
	broken := "---\ntype: plan\ntype: plan\n---\n# Broken\n"
	writeFile(t, path, broken)
	deps, _ := testDependencies("", true, 0)
	err := run(t, deps, "set", "status", "--root", root, "broken", "Implemented")
	if err == nil || !strings.Contains(err.Error(), "does not decode") {
		t.Fatalf("error = %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != broken {
		t.Error("file changed")
	}
}

func TestNewParentAcceptsShortName(t *testing.T) {
	initRepo(t, "upstream", "https://github.com/acme/widgets")
	root := t.TempDir()
	parentFixture(t, root)
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "create", "--root", root, "--parent", "parent-plan", "short"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, strings.TrimSpace(io.stdout.String())); !strings.Contains(got, "\nparent: \"[[260901000000-parent-plan]]\"\n") {
		t.Errorf("content:\n%s", got)
	}
}
