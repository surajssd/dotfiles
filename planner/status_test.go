package main

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

const statusFixture = "---\nType: plan\nParent: \"[[260901000000-parent-plan]]\"\nImplementationStatus: InProgress\nStatusChecked: 2026-09-01\nStatusNote: \"Old note.\"\nowner: me\n---\n\n# Title\n\nBody with status_checked: text that must not change.\n"

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
	want := "---\nType: plan\nParent: \"[[260901000000-parent-plan]]\"\nImplementationStatus: Implemented\nStatusChecked: 2026-09-29\nStatusNote: \"Merged in \\\"PR #7\\\".\"\nowner: me\n---\n\n# Title\n\nBody with status_checked: text that must not change.\n"
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
	if !strings.Contains(readFile(t, path), "ImplementationStatus: InProgress\nStatusChecked: 2026-09-29\nStatusNote: \"Old note.\"") {
		t.Errorf("touch changed more than the date:\n%s", readFile(t, path))
	}
}

func TestStatusNoteOnly(t *testing.T) {
	root, path := statusRoot(t)
	deps, _ := testDependencies("", true, 0)
	if err := run(t, deps, "set", "status", "--root", root, "widgets-plan", "--note", "Still going."); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); !strings.Contains(got, "ImplementationStatus: InProgress\nStatusChecked: 2026-09-29\nStatusNote: \"Still going.\"\n") {
		t.Errorf("content:\n%s", got)
	}
}

func TestStatusNoteFile(t *testing.T) {
	root, path := statusRoot(t)
	noteFile := filepath.Join(t.TempDir(), "note.md")
	writeFile(t, noteFile, "\n\nFirst line.\n\nSecond line.\n\n")
	want := "First line.\n\nSecond line."
	deps, _ := testDependencies("", true, 0)
	if err := run(t, deps, "set", "status", "--root", root, "widgets-plan", "--note-file", noteFile); err != nil {
		t.Fatal(err)
	}
	if front, err := decodeFrontMatter([]byte(readFile(t, path))); err != nil || front.StatusNote != want || front.StatusChecked != "2026-09-29" {
		t.Errorf("from file: front = %+v, error = %v", front, err)
	}
	deps, _ = testDependencies("From stdin.\r\n", false, 0)
	if err := run(t, deps, "set", "status", "--root", root, "widgets-plan", "Implemented", "--note-file", "-"); err != nil {
		t.Fatal(err)
	}
	if front, err := decodeFrontMatter([]byte(readFile(t, path))); err != nil || front.StatusNote != "From stdin." || front.ImplementationStatus != "Implemented" {
		t.Errorf("from stdin: front = %+v, error = %v", front, err)
	}
	before := readFile(t, path)
	writeFile(t, filepath.Join(root, "empty.md"), "\n")
	cases := map[string][]string{
		"not both":      {"--note", "x", "--note-file", noteFile},
		"no such file":  {"--note-file", filepath.Join(root, "missing.md")},
		"holds no text": {"--note-file", filepath.Join(root, "empty.md")},
	}
	for want, args := range cases {
		deps, _ := testDependencies("", true, 0)
		err := run(t, deps, append([]string{"set", "status", "--root", root, "widgets-plan"}, args...)...)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%v: error = %v, want %q", args, err, want)
		}
	}
	if readFile(t, path) != before {
		t.Error("a rejected command changed the file")
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
	want := "---\r\nType: plan\r\nImplementationStatus: Implemented\r\nStatusChecked: 2026-09-29\r\nStatusNote: \"Shipped long ago.\"\r\n---\r\n\r\n# Legacy\r\n\r\nBody.\r\n"
	if got := readFile(t, path); got != want {
		t.Errorf("content:\n%q", got)
	}
}

func TestStatusReplacesBlockScalarNoteAndAppendsMissingKeys(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "github.com", "acme", "widgets", "260913120000-folded.md")
	writeFile(t, path, "---\nType: plan\nStatusNote: >\n  first line\n  second line\nImplementationStatus: InProgress\n---\n# Folded\n")
	deps, _ := testDependencies("", true, 0)
	if err := run(t, deps, "set", "status", "--root", root, "folded", "Superseded", "--note", "Replaced."); err != nil {
		t.Fatal(err)
	}
	want := "---\nType: plan\nStatusNote: \"Replaced.\"\nImplementationStatus: Superseded\nStatusChecked: 2026-09-29\n---\n# Folded\n"
	if got := readFile(t, path); got != want {
		t.Errorf("content:\n%s", got)
	}
}

func TestStatusRefusesBrokenFrontMatter(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "github.com", "acme", "widgets", "260914120000-broken.md")
	broken := "---\nType: plan\nType: plan\n---\n# Broken\n"
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

func TestStatusPreservesYAMLAndNoteText(t *testing.T) {
	for _, note := range []string{
		"StatusNote: |\n  First paragraph.\n\n  Second paragraph.\n",
		"\"StatusNote\": \"Old note.\"\n",
		"StatusNote: \"First line\n  second line\"\n",
	} {
		for _, eol := range []string{"\n", "\r\n"} {
			root, path := statusRoot(t)
			suffix := "# Keep this comment.\nowner: me\n---\n\n# Title\n\nKeep this body.\n"
			input := "---\nType: plan\n\"ImplementationStatus\": InProgress\nStatusChecked: 2026-09-01\n" + note + suffix
			writeFile(t, path, strings.ReplaceAll(input, "\n", eol))
			if err := os.Chmod(path, 0o600); err != nil {
				t.Fatal(err)
			}
			replacement := "First line.\nSecond line.\t\"quoted\" \\path\r\nLast."
			deps, _ := testDependencies("", true, 0)
			if err := run(t, deps, "set", "status", "--root", root, "widgets-plan", "Implemented", "--note", replacement); err != nil {
				t.Fatal(err)
			}
			got := readFile(t, path)
			front, err := decodeFrontMatter([]byte(got))
			if err != nil || front.StatusNote != replacement || front.ImplementationStatus != "Implemented" {
				t.Errorf("note %q: front = %+v, error = %v", note, front, err)
			}
			if !strings.HasSuffix(got, strings.ReplaceAll(suffix, "\n", eol)) {
				t.Errorf("unrelated content changed: %q", got)
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("file permissions changed: %v, %v", info, err)
			}
		}
	}
}

func TestMetadataWriteFailureKeepsPlan(t *testing.T) {
	if root := os.Getenv("PLANNER_TEST_WRITE_ROOT"); root != "" {
		signal.Ignore(syscall.SIGXFSZ)
		if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &syscall.Rlimit{Cur: 96, Max: 96}); err != nil {
			t.Fatal(err)
		}
		args := []string{"set", "status", "--root", root, "widgets-plan", "--note", "Updated."}
		if os.Getenv("PLANNER_TEST_WRITE_KIND") == "issue" {
			args = []string{"set", "issue", "--root", root, "widgets-plan", "https://example.com/ticket/1"}
		}
		deps, _ := testDependencies("", true, 0)
		if err := run(t, deps, args...); err == nil || !strings.Contains(err.Error(), "file too large") {
			t.Fatalf("write error = %v", err)
		}
		return
	}
	for _, kind := range []string{"status", "issue"} {
		root, path := statusRoot(t)
		cmd := exec.Command(os.Args[0], "-test.run=^TestMetadataWriteFailureKeepsPlan$")
		cmd.Env = append(os.Environ(), "PLANNER_TEST_WRITE_ROOT="+root, "PLANNER_TEST_WRITE_KIND="+kind)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", kind, err, out)
		}
		if got := readFile(t, path); got != statusFixture {
			t.Errorf("%s: failed write changed the plan: %q", kind, got)
		}
		if files := listFiles(t, root); len(files) != 1 {
			t.Errorf("%s: temporary files remain: %v", kind, files)
		}
	}
}

func TestStatusKeepsPlanWhenReplacementBreaksAnAlias(t *testing.T) {
	root, path := statusRoot(t)
	input := "---\nType: plan\nImplementationStatus: InProgress\nStatusChecked: 2026-09-01\nStatusNote: &note https://example.com/parent\nParent: *note\n---\n# Title\n"
	writeFile(t, path, input)
	deps, _ := testDependencies("", true, 0)
	err := run(t, deps, "set", "status", "--root", root, "widgets-plan", "--note", "Changed.")
	if err == nil || !strings.Contains(err.Error(), "does not decode") {
		t.Fatalf("invalid replacement error = %v", err)
	}
	if got := readFile(t, path); got != input {
		t.Errorf("rejected replacement changed the file: %q", got)
	}
}

func TestLinkEditsPreserveCommentsAndUnrelatedFields(t *testing.T) {
	root, path := statusRoot(t)
	input := strings.Replace(statusFixture, "owner: me\n", "\"Issues\":\n  - https://example.com/one\n\n# Keep this comment.\nowner: me\n", 1)
	writeFile(t, path, input)
	deps, _ := testDependencies("", true, 0)
	if err := run(t, deps, "set", "issue", "--root", root, "widgets-plan", "https://example.com/two"); err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(input, "\"Issues\":\n  - https://example.com/one\n", "Issues:\n  - https://example.com/one\n  - https://example.com/two\n", 1)
	if got := readFile(t, path); got != want {
		t.Errorf("link update changed unrelated text: %q", got)
	}
}

const otherFixture = "---\nType: plan\nImplementationStatus: InProgress\nStatusChecked: 2026-09-01\nStatusNote: \"Other.\"\n---\n\n# Other plan\n"

func TestSetParentStoresWikilinkAndLeavesDateAlone(t *testing.T) {
	root, path := statusRoot(t)
	writeFile(t, filepath.Join(root, "github.com", "acme", "gadgets", "260902000000-other-plan.md"), otherFixture)
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "set", "parent", "--root", root, "widgets-plan", "other-plan"); err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(statusFixture, "Parent: \"[[260901000000-parent-plan]]\"", "Parent: \"[[260902000000-other-plan]]\"", 1)
	if got := readFile(t, path); got != want {
		t.Errorf("content:\n%s", got)
	}
	if out := io.stdout.String(); out != "Parent: [[260902000000-other-plan]]\n" {
		t.Errorf("output: %q", out)
	}
	deps, io = testDependencies("", true, 0)
	if err := run(t, deps, "tree", "--root", root); err != nil {
		t.Fatal(err)
	}
	if out := io.stdout.String(); !strings.Contains(out, "other-plan") || !strings.Contains(out, "└── widgets-plan") {
		t.Errorf("tree does not nest the plan under its new parent:\n%s", out)
	}
}

func TestSetParentStoresPathsAndURLsAsGiven(t *testing.T) {
	root, path := statusRoot(t)
	outside := filepath.Join(t.TempDir(), "notes.md")
	writeFile(t, outside, "# Notes\n")
	for _, ref := range []string{"https://github.com/acme/widgets/issues/1", outside} {
		deps, _ := testDependencies("", true, 0)
		if err := run(t, deps, "set", "parent", "--root", root, "widgets-plan", ref); err != nil {
			t.Fatalf("%s: %v", ref, err)
		}
		front, err := decodeFrontMatter([]byte(readFile(t, path)))
		if err != nil || front.Parent != ref || front.StatusChecked != "2026-09-01" {
			t.Errorf("%s: front = %+v, error = %v", ref, front, err)
		}
	}
}

func TestSetParentAddsMissingKey(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "github.com", "acme", "widgets", "260910120000-orphan.md")
	writeFile(t, path, "---\nType: plan\nImplementationStatus: InProgress\nStatusChecked: 2026-09-01\nStatusNote: \"Orphan.\"\n---\n# Orphan\n")
	parentFixture(t, root)
	deps, _ := testDependencies("", true, 0)
	if err := run(t, deps, "set", "parent", "--root", root, "orphan", "parent-plan"); err != nil {
		t.Fatal(err)
	}
	want := "---\nType: plan\nImplementationStatus: InProgress\nStatusChecked: 2026-09-01\nStatusNote: \"Orphan.\"\nParent: \"[[260901000000-parent-plan]]\"\n---\n# Orphan\n"
	if got := readFile(t, path); got != want {
		t.Errorf("content:\n%s", got)
	}
}

func TestSetParentRejectsBadInput(t *testing.T) {
	root, path := statusRoot(t)
	parentPath := parentFixture(t, root)
	parentContent := readFile(t, parentPath)
	writeFile(t, filepath.Join(root, "github.com", "acme", "widgets", "260912120000-legacy.md"), "# Legacy\n")
	cases := map[string][]string{
		"its own parent":  {"widgets-plan", "widgets-plan"},
		"cycle":           {"parent-plan", "widgets-plan"},
		"not found":       {"widgets-plan", "nope"},
		"does not exist":  {"widgets-plan", "/no/such/notes.md"},
		"no front matter": {"legacy", "widgets-plan"},
	}
	for want, args := range cases {
		deps, _ := testDependencies("", true, 0)
		err := run(t, deps, append([]string{"set", "parent", "--root", root}, args...)...)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%v: error = %v, want %q", args, err, want)
		}
	}
	if readFile(t, path) != statusFixture || readFile(t, parentPath) != parentContent {
		t.Error("a rejected command changed a file")
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
	if got := readFile(t, strings.TrimSpace(io.stdout.String())); !strings.Contains(got, "\nParent: \"[[260901000000-parent-plan]]\"\n") {
		t.Errorf("content:\n%s", got)
	}
}
