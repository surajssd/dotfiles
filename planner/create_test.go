package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const generatedHead = "---\nType: plan\nImplementationStatus: NotImplemented\nStatusChecked: 2026-09-29\nStatusNote: \"Implementation has not started.\"\n---\n\n"

// initRepo creates a Git repository with one remote and changes into it.
func initRepo(t *testing.T, remote, url string) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", remote, url}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	t.Chdir(dir)
	return dir
}

// fakeGh puts a gh script first on PATH that prints the given JSON, or fails
// when json is empty.
func fakeGh(t *testing.T, json string) {
	t.Helper()
	bin := t.TempDir()
	script := "#!/bin/sh\nexit 1\n"
	if json != "" {
		script = "#!/bin/sh\ncat >/dev/null\nprintf '%s\\n' '" + json + "'\n"
	}
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func listFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, path)
		}
		return err
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return files
}

func TestNewUsesUpstreamAndPipedBody(t *testing.T) {
	initRepo(t, "upstream", "git@github.com:acme/widgets.git")
	root := t.TempDir()
	deps, io := testDependencies("# My plan\n\nBody.\n", false, 0)
	if err := run(t, deps, "create", "--root", root, "my", "plan words.md"); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "github.com", "acme", "widgets", "260929120000-my-plan-words.md")
	if io.stdout.String() != want+"\n" {
		t.Errorf("stdout = %q, want %q", io.stdout.String(), want)
	}
	if got := readFile(t, want); got != generatedHead+"# My plan\n\nBody.\n" {
		t.Errorf("content:\n%s", got)
	}
	if io.stderr.Len() != 0 {
		t.Errorf("stderr: %s", io.stderr)
	}
}

func TestNewTerminalStdinWritesFrontMatterOnly(t *testing.T) {
	initRepo(t, "upstream", "https://github.com/acme/widgets")
	root := t.TempDir()
	deps, _ := testDependencies("must not be read", true, 0)
	if err := run(t, deps, "create", "--root", root, "empty"); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(root, "github.com", "acme", "widgets", "260929120000-empty.md"))
	if got != generatedHead {
		t.Errorf("content:\n%s", got)
	}
}

func TestNewMapsForkToParentThroughGh(t *testing.T) {
	initRepo(t, "origin", "ssh://git@github.com/fork/widgets.git")
	fakeGh(t, `{"isFork":true,"parent":{"owner":{"login":"acme"},"name":"widgets"},"nameWithOwner":"fork/widgets"}`)
	root := t.TempDir()
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "create", "--root", root, "forked"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(io.stdout.String(), filepath.Join("github.com", "acme", "widgets")) {
		t.Errorf("stdout = %q", io.stdout)
	}
}

func TestNewFallsBackToOriginWithWarning(t *testing.T) {
	initRepo(t, "origin", "https://github.com/fork/widgets.git")
	fakeGh(t, "")
	root := t.TempDir()
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "create", "--root", root, "fallback"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(io.stdout.String(), filepath.Join("github.com", "fork", "widgets")) {
		t.Errorf("stdout = %q", io.stdout)
	}
	if !strings.Contains(io.stderr.String(), "using origin (fork/widgets)") {
		t.Errorf("stderr = %q", io.stderr)
	}
}

func TestNewOutsideGitFails(t *testing.T) {
	t.Chdir(t.TempDir())
	deps, _ := testDependencies("", true, 0)
	err := run(t, deps, "create", "--root", t.TempDir(), "nowhere")
	if err == nil || !strings.Contains(err.Error(), "Git checkout") {
		t.Fatalf("error = %v", err)
	}
}

func TestNewReadsRootFromConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFile(t, filepath.Join(home, configName), "root: ~/plans\n")
	initRepo(t, "upstream", "https://github.com/acme/widgets")
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "create", "configured"); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "plans", "github.com", "acme", "widgets", "260929120000-configured.md")
	if io.stdout.String() != want+"\n" {
		t.Errorf("stdout = %q, want %q", io.stdout.String(), want)
	}
}

type failingReader struct{ t *testing.T }

func (r failingReader) Read([]byte) (int, error) {
	r.t.Error("stdin was read")
	return 0, io.EOF
}

func TestNewCollisionKeepsFileAndSkipsStdin(t *testing.T) {
	initRepo(t, "upstream", "https://github.com/acme/widgets")
	root := t.TempDir()
	existing := filepath.Join(root, "github.com", "acme", "widgets", "260929120000-taken.md")
	writeFile(t, existing, "original\n")
	deps, _ := testDependencies("", false, 0)
	deps.stdin = failingReader{t}
	err := run(t, deps, "create", "--root", root, "taken")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error = %v", err)
	}
	if readFile(t, existing) != "original\n" {
		t.Error("existing file was changed")
	}
}

func TestNewRejectsBadNames(t *testing.T) {
	initRepo(t, "upstream", "https://github.com/acme/widgets")
	root := t.TempDir()
	for _, args := range [][]string{{}, {""}, {"  "}, {"."}, {".."}, {"a/b"}, {`a\b`}, {".md"}} {
		deps, _ := testDependencies("", true, 0)
		if err := run(t, deps, append([]string{"create", "--root", root}, args...)...); err == nil {
			t.Errorf("args %q were accepted", args)
		}
	}
	if files := listFiles(t, root); len(files) != 0 {
		t.Errorf("files were created: %v", files)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("stdin broke") }

func TestNewRemovesFileAfterInputFailure(t *testing.T) {
	initRepo(t, "upstream", "https://github.com/acme/widgets")
	root := t.TempDir()
	deps, _ := testDependencies("", false, 0)
	deps.stdin = errReader{}
	err := run(t, deps, "create", "--root", root, "broken")
	if err == nil || !strings.Contains(err.Error(), "stdin broke") {
		t.Fatalf("error = %v", err)
	}
	if files := listFiles(t, root); len(files) != 0 {
		t.Errorf("files were left behind: %v", files)
	}
}

// parentFixture dumps a plan for a second repository under root and returns
// its path, so a child created for acme/widgets crosses repositories.
func parentFixture(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "github.com", "acme", "gadgets", "260901000000-parent-plan.md")
	writeFile(t, path, "---\nType: plan\nImplementationStatus: InProgress\nStatusChecked: 2026-09-01\nStatusNote: \"Parent.\"\n---\n\n# Parent plan\n")
	return path
}

func TestNewParentFormsStoreWikilink(t *testing.T) {
	initRepo(t, "upstream", "https://github.com/acme/widgets")
	root := t.TempDir()
	parentPath := parentFixture(t, root)
	refs := map[string]string{
		"bare":     "260901000000-parent-plan",
		"with-md":  "260901000000-parent-plan.md",
		"wikilink": "[[260901000000-parent-plan]]",
		"absolute": parentPath,
		"home":     "~" + strings.TrimPrefix(parentPath, os.Getenv("HOME")),
	}
	if !strings.HasPrefix(parentPath, os.Getenv("HOME")) {
		delete(refs, "home")
	}
	for name, ref := range refs {
		deps, io := testDependencies("# Child\n", false, 0)
		if err := run(t, deps, "create", "--root", root, "--parent", ref, "child", name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := readFile(t, strings.TrimSpace(io.stdout.String()))
		want := "---\nType: plan\nParent: \"[[260901000000-parent-plan]]\"\nImplementationStatus"
		if !strings.HasPrefix(got, want) {
			t.Errorf("%s: content starts with:\n%s", name, got[:min(len(got), len(want)+20)])
		}
	}
}

func TestNewExternalParentsAreStoredAsWritten(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFile(t, filepath.Join(home, ".claude", "plans", "umbrella.md"), "# Umbrella\n")
	initRepo(t, "upstream", "https://github.com/acme/widgets")
	root := t.TempDir()
	external := filepath.Join(t.TempDir(), "design.md")
	writeFile(t, external, "# Design\n")
	cases := map[string]string{
		"~/.claude/plans/umbrella.md": "~/.claude/plans/umbrella.md",
		external:                      external,
		"https://github.com/acme/widgets/issues/7": "https://github.com/acme/widgets/issues/7",
	}
	i := 0
	for ref, want := range cases {
		i++
		deps, io := testDependencies("", true, 0)
		if err := run(t, deps, "create", "--root", root, "--parent", ref, "external", string(rune('a'+i))); err != nil {
			t.Fatalf("%s: %v", ref, err)
		}
		got := readFile(t, strings.TrimSpace(io.stdout.String()))
		if !strings.Contains(got, "\nParent: \""+want+"\"\n") {
			t.Errorf("%s stored as:\n%s", ref, got)
		}
	}
}

func TestNewRelativeExternalParentIsStoredAbsolute(t *testing.T) {
	checkout := initRepo(t, "upstream", "https://github.com/acme/widgets")
	writeFile(t, filepath.Join(checkout, "docs", "umbrella.md"), "# Umbrella\n")
	root := t.TempDir()
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "create", "--root", root, "--parent", "./docs/umbrella.md", "relative"); err != nil {
		t.Fatal(err)
	}
	child := strings.TrimSpace(io.stdout.String())
	want := filepath.Join(checkout, "docs", "umbrella.md")
	if !strings.Contains(readFile(t, child), "\nParent: \""+want+"\"\n") {
		t.Errorf("content:\n%s", readFile(t, child))
	}
	c, err := loadCorpus(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	for _, f := range checkCorpus(c) {
		if f.rule == "parent-not-found" {
			t.Errorf("saved child does not resolve its parent: %s", f.detail)
		}
	}
}

func TestNewParentValidationHappensBeforeCreation(t *testing.T) {
	initRepo(t, "upstream", "https://github.com/acme/widgets")
	root := t.TempDir()
	parentPath := parentFixture(t, root)
	writeFile(t, filepath.Join(root, "github.com", "acme", "gizmos", "260901000000-parent-plan.md"), "# Twin\n")
	before := listFiles(t, root)
	cases := map[string]string{
		parentPath:                     "ambiguous",
		"[[260901000000-parent-plan]]": "ambiguous",
		"[[260999000000-nope]]":        "not found",
		"/no/such/file.md":             "does not exist",
	}
	for ref, want := range cases {
		deps, _ := testDependencies("", false, 0)
		deps.stdin = failingReader{t}
		err := run(t, deps, "create", "--root", root, "--parent", ref, "child")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: error = %v, want %q", ref, err, want)
		}
	}
	if after := listFiles(t, root); len(after) != len(before) {
		t.Errorf("files changed: %v", after)
	}
}

func TestNewPreservesPipedFrontMatter(t *testing.T) {
	initRepo(t, "upstream", "https://github.com/acme/widgets")
	root := t.TempDir()
	piped := "---\nType: plan\nImplementationStatus: InProgress\nStatusChecked: 2026-09-01\nStatusNote: \"Mine.\"\n---\n\n# Piped\n"
	deps, io := testDependencies(piped, false, 0)
	if err := run(t, deps, "create", "--root", root, "piped plain"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, strings.TrimSpace(io.stdout.String())); got != piped {
		t.Errorf("content:\n%s", got)
	}

	deps, io = testDependencies(piped, false, 0)
	if err := run(t, deps, "create", "--root", root, "--parent", "https://example.com/x", "piped parent"); err != nil {
		t.Fatal(err)
	}
	want := "---\nParent: \"https://example.com/x\"\nType: plan\n"
	if got := readFile(t, strings.TrimSpace(io.stdout.String())); !strings.HasPrefix(got, want) {
		t.Errorf("content:\n%s", got)
	}

	before := listFiles(t, root)
	withParent := "---\nType: plan\nParent: \"[[x]]\"\n---\n"
	deps, _ = testDependencies(withParent, false, 0)
	err := run(t, deps, "create", "--root", root, "--parent", "https://example.com/x", "piped duplicate")
	if err == nil || !strings.Contains(err.Error(), "already has a parent") {
		t.Fatalf("error = %v", err)
	}
	if after := listFiles(t, root); len(after) != len(before) {
		t.Errorf("a file was left behind: %v", after)
	}
}

func TestPlanName(t *testing.T) {
	cases := map[string][]string{
		"improve-performance": {"improve", "performance"},
		"a-b-c":               {"a b", " c "},
		"trailing":            {"trailing.md"},
		"keep":                {"keep", ".md"},
	}
	for want, args := range cases {
		got, err := planName(args)
		if err != nil || got != want {
			t.Errorf("planName(%q) = %q, %v; want %q", args, got, err, want)
		}
	}
}

func TestGithubRepoFromURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/acme/widgets.git":   "acme/widgets",
		"https://github.com/acme/widgets/":      "acme/widgets",
		"git@github.com:acme/widgets.git":       "acme/widgets",
		"ssh://git@github.com:22/acme/widgets":  "acme/widgets",
		"ssh://github.com/acme/wid.gets":        "acme/wid.gets",
		"https://gitlab.com/acme/widgets.git":   "",
		"https://github.com/acme":               "",
		"https://github.com/acme/..":            "",
		"git@github.com:acme/widgets/extra.git": "",
	}
	for url, want := range cases {
		got, ok := githubRepoFromURL(url)
		if got != want || ok != (want != "") {
			t.Errorf("githubRepoFromURL(%q) = %q, %v", url, got, ok)
		}
	}
}

func TestNewStatusAndNoteFlags(t *testing.T) {
	initRepo(t, "upstream", "https://github.com/acme/widgets")
	root := t.TempDir()
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "create", "--root", root, "--status", "in-progress", "--note", "Started today.", "flags"); err != nil {
		t.Fatal(err)
	}
	want := "---\nType: plan\nImplementationStatus: InProgress\nStatusChecked: 2026-09-29\nStatusNote: \"Started today.\"\n---\n\n"
	if got := readFile(t, strings.TrimSpace(io.stdout.String())); got != want {
		t.Errorf("content:\n%s", got)
	}

	deps, io = testDependencies("", true, 0)
	if err := run(t, deps, "create", "--root", root, "--note", "Only a note.", "note only"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, strings.TrimSpace(io.stdout.String())); !strings.Contains(got, "ImplementationStatus: NotImplemented\nStatusChecked: 2026-09-29\nStatusNote: \"Only a note.\"\n") {
		t.Errorf("content:\n%s", got)
	}

	piped := "---\nType: plan\nImplementationStatus: NotImplemented\nStatusChecked: 2026-09-01\nStatusNote: \"Old.\"\n---\n\n# Piped\n"
	deps, io = testDependencies(piped, false, 0)
	if err := run(t, deps, "create", "--root", root, "--status", "Superseded", "--note", "Replaced.", "--parent", "https://example.com/p", "piped flags"); err != nil {
		t.Fatal(err)
	}
	want = "---\nParent: \"https://example.com/p\"\nType: plan\nImplementationStatus: Superseded\nStatusChecked: 2026-09-29\nStatusNote: \"Replaced.\"\n---\n\n# Piped\n"
	if got := readFile(t, strings.TrimSpace(io.stdout.String())); got != want {
		t.Errorf("content:\n%s", got)
	}

	before := listFiles(t, root)
	cases := map[string][]string{
		"needs --note":   {"--status", "Implemented"},
		"unknown status": {"--status", "done", "--note", "x"},
	}
	for want, flags := range cases {
		deps, _ := testDependencies("", true, 0)
		err := run(t, deps, append([]string{"create", "--root", root, "rejected"}, flags...)...)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%v: error = %v, want %q", flags, err, want)
		}
	}
	if after := listFiles(t, root); len(after) != len(before) {
		t.Errorf("a rejected command created a file: %v", after)
	}
}
