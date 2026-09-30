package main

import (
	"path/filepath"
	"strings"
	"testing"
)

const logFront = "---\nType: plan\nImplementationStatus: InProgress\nStatusChecked: 2026-09-01\nStatusNote: \"Old note.\"\n---\n"

func logRoot(t *testing.T, body string) (string, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "github.com", "acme", "widgets", "260910120000-widgets-plan.md")
	writeFile(t, path, logFront+body)
	return root, path
}

const bumpedFront = "---\nType: plan\nImplementationStatus: InProgress\nStatusChecked: 2026-09-29\nStatusNote: \"Old note.\"\n---\n"

func TestLogAppendsToExistingSectionBeforeNextHeading(t *testing.T) {
	body := "\n# Title\n\n## Summary\n\nText.\n\n## Progress log\n\n### 2026-09-01: first\n\nOld entry.\n\n\n## Risks\n\nRisk text.\n"
	root, path := logRoot(t, body)
	deps, io := testDependencies("New body.\n\nSecond paragraph.\n", false, 0)
	if err := run(t, deps, "log", "--root", root, "widgets-plan", "PR", "1", "merged"); err != nil {
		t.Fatal(err)
	}
	want := bumpedFront + "\n# Title\n\n## Summary\n\nText.\n\n## Progress log\n\n### 2026-09-01: first\n\nOld entry.\n\n### 2026-09-29: PR 1 merged\n\nNew body.\n\nSecond paragraph.\n\n## Risks\n\nRisk text.\n"
	if got := readFile(t, path); got != want {
		t.Errorf("content:\n%s", got)
	}
	if out := io.stdout.String(); out != "### 2026-09-29: PR 1 merged\n" {
		t.Errorf("output: %q", out)
	}
}

func TestLogCreatesSectionAtEndOfFile(t *testing.T) {
	cases := map[string]string{
		"trailing newline":    "\n# Title\n\nText.\n\n\n",
		"no trailing newline": "\n# Title\n\nText.",
		"front matter only":   "",
	}
	for name, body := range cases {
		root, path := logRoot(t, body)
		deps, _ := testDependencies("must not be read", true, 0)
		if err := run(t, deps, "log", "--root", root, "widgets-plan", "Started"); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want := bumpedFront + strings.TrimRight(body, "\n") + "\n\n## Progress log\n\n### 2026-09-29: Started\n"
		if body == "" {
			want = bumpedFront + "\n## Progress log\n\n### 2026-09-29: Started\n"
		}
		if got := readFile(t, path); got != want {
			t.Errorf("%s:\n%s", name, got)
		}
	}
}

func TestLogIgnoresHeadingsInFencesAndMatchesCase(t *testing.T) {
	body := "# Title\n\n## Progress Log\n\n### 2026-09-01: first\n\n```md\n## Not a heading\n```\n\n~~~\n# Nor this\n~~~\n\n## Next\n"
	root, path := logRoot(t, body)
	deps, _ := testDependencies("", true, 0)
	if err := run(t, deps, "log", "--root", root, "widgets-plan", "second"); err != nil {
		t.Fatal(err)
	}
	want := bumpedFront + "# Title\n\n## Progress Log\n\n### 2026-09-01: first\n\n```md\n## Not a heading\n```\n\n~~~\n# Nor this\n~~~\n\n### 2026-09-29: second\n\n## Next\n"
	if got := readFile(t, path); got != want {
		t.Errorf("content:\n%s", got)
	}
}

func TestLogKeepsCRLFAndReplacesNote(t *testing.T) {
	root, path := logRoot(t, "")
	writeFile(t, path, strings.ReplaceAll(logFront+"\n# Title\n\n## Progress log\n\n### 2026-09-01: first\n", "\n", "\r\n"))
	deps, _ := testDependencies("Line one.\nLine two.\n", false, 0)
	if err := run(t, deps, "log", "--root", root, "widgets-plan", "second", "--note", "Short note."); err != nil {
		t.Fatal(err)
	}
	want := strings.ReplaceAll("---\nType: plan\nImplementationStatus: InProgress\nStatusChecked: 2026-09-29\nStatusNote: \"Short note.\"\n---\n\n# Title\n\n## Progress log\n\n### 2026-09-01: first\n\n### 2026-09-29: second\n\nLine one.\nLine two.\n", "\n", "\r\n")
	if got := readFile(t, path); got != want {
		t.Errorf("content:\n%q", got)
	}
}

func TestLogRejectsBadInput(t *testing.T) {
	root, path := logRoot(t, "# Title\n")
	legacy := filepath.Join(root, "github.com", "acme", "widgets", "260911120000-legacy.md")
	writeFile(t, legacy, "# Legacy\n")
	broken := filepath.Join(root, "github.com", "acme", "widgets", "260912120000-broken.md")
	writeFile(t, broken, "---\nType: plan\nType: plan\n---\n# Broken\n")
	cases := map[string][]string{
		"nonempty entry title": {"widgets-plan", " "},
		"no front matter":      {"legacy", "Started"},
		"does not decode":      {"broken", "Started"},
		"not found":            {"nope", "Started"},
	}
	for want, args := range cases {
		deps, _ := testDependencies("", true, 0)
		err := run(t, deps, append([]string{"log", "--root", root}, args...)...)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%v: error = %v, want %q", args, err, want)
		}
	}
	for _, file := range []string{path, legacy, broken} {
		if got := readFile(t, file); strings.Contains(got, "2026-09-29") {
			t.Errorf("a rejected command changed %s:\n%s", file, got)
		}
	}
}
