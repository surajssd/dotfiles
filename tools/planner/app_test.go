package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite golden files")

var fixedNow = time.Date(2026, time.September, 29, 12, 0, 0, 0, time.Local)

type testIO struct {
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func testDependencies(stdin string, stdinIsTerminal bool, width int) (dependencies, testIO) {
	io := testIO{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	return dependencies{
		stdin:           strings.NewReader(stdin),
		stdout:          io.stdout,
		stderr:          io.stderr,
		now:             func() time.Time { return fixedNow },
		stdinIsTerminal: func() bool { return stdinIsTerminal },
		stdoutWidth:     func() (int, bool) { return width, width > 0 },
	}, io
}

// run executes planner with exactly the given arguments. An empty, non-nil
// slice keeps cobra from falling back to the test binary's own os.Args.
func run(t *testing.T, deps dependencies, args ...string) error {
	t.Helper()
	if args == nil {
		args = []string{}
	}
	cmd := newCommand(deps)
	cmd.SetArgs(args)
	return cmd.Execute()
}

// fixtureHome creates a HOME that holds the ~/.claude/plans file the fixture
// corpus points at.
func fixtureHome(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	writeFile(t, filepath.Join(home, ".claude", "plans", "widgets-recipe.md"), "# Widgets recipe\n")
	return home
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixtureRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("testdata/plans")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		writeFile(t, path, got)
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("%s differs\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestListGolden(t *testing.T) {
	home := fixtureHome(t)
	root := filepath.Join(home, "plans")
	for _, dir := range []string{"plans", "external"} {
		if err := os.CopyFS(filepath.Join(home, dir), os.DirFS(filepath.Join("testdata", dir))); err != nil {
			t.Fatal(err)
		}
	}
	// The tree shows the legacy parent of an active child, so it hides one
	// plan fewer than the flat list.
	hiddenTree := "3 plans without valid front matter hidden (--all shows them; planner check lists the problems)\n"
	hiddenGet := "4 plans without valid front matter hidden (--all shows them; planner check lists the problems)\n"
	cases := []struct {
		name   string
		width  int
		args   []string
		stderr string
	}{
		{"tree-active.golden", 0, []string{"tree"}, hiddenTree},
		{"tree-all.golden", 0, []string{"tree", "--all"}, ""},
		{"tree-wide.golden", 0, []string{"tree", "-o", "wide", "--all"}, ""},
		{"tree-repo.golden", 0, []string{"tree", "--repo", "acme/gadgets", "--all"}, ""},
		{"tree-status.golden", 0, []string{"tree", "--status", "implemented"}, ""},
		{"tree-narrow.golden", 130, []string{"tree"}, hiddenTree},
		{"get-active.golden", 0, []string{"get"}, hiddenGet},
		{"get-repos.golden", 0, []string{"get", "repos"}, ""},
		{"get-all-wide.golden", 0, []string{"get", "--all", "-o", "wide"}, ""},
		{"get-json-single.golden", 0, []string{"get", "-o", "json", "widgets-umbrella"}, ""},
		{"get-json-list.golden", 0, []string{"get", "-o", "json", "--repo", "gadgets", "--all"}, ""},
		{"get-yaml.golden", 0, []string{"get", "-o", "yaml", "widgets-umbrella", "widgets-legacy"}, ""},
		{"get-name.golden", 0, []string{"get", "-o", "name", "--all"}, ""},
		{"get-no-headers.golden", 0, []string{"get", "--no-headers"}, hiddenGet},
		{"tree-no-headers.golden", 0, []string{"tree", "--no-headers", "-o", "wide", "widgets-umbrella"}, ""},
		{"get-repo.golden", 0, []string{"get", "--repo", "GADGETS", "--all"}, ""},
		{"get-status.golden", 0, []string{"get", "--status", "not-implemented"}, ""},
		{"get-named.golden", 0, []string{"get", "widgets-child-done", "[[260915120000-widgets-legacy]]"}, ""},
		{"tree-named.golden", 0, []string{"tree", "widgets-umbrella", "widgets-done-umbrella"}, ""},
		{"tree-named-all.golden", 0, []string{"tree", "widgets-umbrella", "--all"}, ""},
		{"describe.golden", 0, []string{"describe", "widgets-umbrella", "widgets-legacy", "260918120000-widgets-links", "widgets-links-fields", "widgets-superseded"}, ""},
		{"get-url.golden", 0, []string{"get", "https://acme.atlassian.net/browse/WID-1"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps, io := testDependencies("", true, tc.width)
			if err := run(t, deps, append(tc.args, "--root", root)...); err != nil {
				t.Fatal(err)
			}
			got := strings.NewReplacer(root, "<root>", "~/plans", "<root>").Replace(io.stdout.String())
			assertGolden(t, tc.name, got)
			if io.stderr.String() != tc.stderr {
				t.Errorf("stderr = %q, want %q", io.stderr, tc.stderr)
			}
		})
	}
}

func TestRepoFilterIsPartialAndCaseInsensitive(t *testing.T) {
	fixtureHome(t)
	root := fixtureRoot(t)
	for _, cmd := range []string{"get", "tree"} {
		for _, repo := range []string{"gadgets", "Acme/GADGETS", "adget"} {
			deps, io := testDependencies("", true, 0)
			if err := run(t, deps, cmd, "--root", root, "--repo", repo, "--all"); err != nil {
				t.Fatal(err)
			}
			out := io.stdout.String()
			if !strings.Contains(out, "gadgets-cross-child") || strings.Contains(out, "widgets-crlf") || strings.Contains(out, "acme/gizmos") {
				t.Errorf("%s --repo %q printed:\n%s", cmd, repo, out)
			}
		}
		deps, io := testDependencies("", true, 0)
		if err := run(t, deps, cmd, "--root", root, "--repo", "nomatch", "--all"); err != nil {
			t.Fatal(err)
		}
		if io.stdout.Len() != 0 || io.stderr.String() != "No plans found.\n" {
			t.Errorf("%s --repo nomatch: stdout = %q, stderr = %q", cmd, io.stdout, io.stderr)
		}
	}
}

func TestRepoExclusion(t *testing.T) {
	fixtureHome(t)
	root := fixtureRoot(t)
	list := func(t *testing.T, args ...string) testIO {
		t.Helper()
		deps, io := testDependencies("", true, 0)
		if err := run(t, deps, append([]string{"--root", root}, args...)...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return io
	}
	for _, cmd := range []string{"get", "tree"} {
		for _, repo := range []string{"!=acme/widgets", "!=widgets", "!=ACME/Widgets", "!= acme/widgets "} {
			out := list(t, cmd, "--repo", repo, "--all").stdout.String()
			for _, want := range []string{"loose-plan", "gadgets-cross-child", "acme/gizmos"} {
				if !strings.Contains(out, want) {
					t.Errorf("%s --repo %q lacks %q:\n%s", cmd, repo, want, out)
				}
			}
			// The tree places gadgets-cross-child under its excluded parent.
			if strings.Contains(out, "widgets-crlf") || strings.Contains(out, "widgets-umbrella") != (cmd == "tree") {
				t.Errorf("%s --repo %q printed:\n%s", cmd, repo, out)
			}
		}
		all := list(t, cmd, "--all")
		unmatched := list(t, cmd, "--repo", "!=nomatch", "--all")
		if unmatched.stdout.String() != all.stdout.String() || unmatched.stderr.String() != all.stderr.String() {
			t.Errorf("%s --repo !=nomatch differs from --all:\n%s", cmd, unmatched.stdout)
		}
		out := list(t, cmd, "--repo", "!=widgets", "--status", "implemented").stdout.String()
		if !strings.Contains(out, "Shared name in gadgets") || !strings.Contains(out, "Shared name in gizmos") || strings.Contains(out, "widgets-") {
			t.Errorf("%s --repo !=widgets --status implemented printed:\n%s", cmd, out)
		}
		hidden := map[string]string{"get": "4", "tree": "3"}[cmd] + " plans without valid front matter hidden (--all shows them; planner check lists the problems)\n"
		if got := list(t, cmd, "--repo", "!=gadgets").stderr.String(); got != hidden {
			t.Errorf("%s --repo !=gadgets: stderr = %q, want %q", cmd, got, hidden)
		}
		if got := list(t, cmd, "--repo", "!=widgets").stderr.String(); got != "" {
			t.Errorf("%s --repo !=widgets: stderr = %q", cmd, got)
		}
		if out := list(t, cmd, "--repo", "!=widgets", "widgets-crlf").stdout.String(); !strings.Contains(out, "widgets-crlf") {
			t.Errorf("%s --repo !=widgets widgets-crlf printed:\n%s", cmd, out)
		}
	}
	io := list(t, "get", "-o", "json", "--repo", "!=acme", "--status", "implemented")
	if got := io.stdout.String(); got != "{\n    \"items\": []\n}\n" || io.stderr.Len() != 0 {
		t.Errorf("empty json list: stdout = %q, stderr = %q", got, io.stderr)
	}
}

func TestRepoFilterRejectsBareExclusion(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	for _, cmd := range []string{"get", "tree"} {
		for _, repo := range []string{"!=", "!= "} {
			deps, io := testDependencies("", true, 0)
			err := run(t, deps, cmd, "--root", root, "--repo", repo)
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("--repo %q names no repository", repo)) || io.stdout.Len() != 0 {
				t.Errorf("%s --repo %q: error = %v, stdout = %q", cmd, repo, err, io.stdout)
			}
		}
	}
}

func TestGetByPullRequest(t *testing.T) {
	root := t.TempDir()
	for _, file := range []struct{ repo, name, status, fields string }{
		{"widgets", "active", "InProgress", "PullRequests: [" + prA + ", " + prA + "]\n"},
		{"widgets", "done", "Implemented", "PullRequests: [\" " + prA + " \"]\n"},
		{"gadgets", "abandoned", "Abandoned", "PullRequests: [" + prA + "]\n"},
		{"gadgets", "superseded", "Superseded", "PullRequests: [" + prA + "]\n"},
		{"widgets", "other-pr", "InProgress", "PullRequests: [" + prB + "]\n"},
		{"widgets", "parent-only", "InProgress", "Parent: " + prA + "\n"},
		{"widgets", "successor-only", "InProgress", "SupersededBy: " + prA + "\n"},
		{"widgets", "issue-only", "InProgress", "Issues: [" + prA + "]\n"},
		{"widgets", "note-only", "InProgress", ""},
	} {
		content := "---\nType: plan\nImplementationStatus: " + file.status + "\nStatusChecked: 2026-09-01\nStatusNote: " + prA + "\n" + file.fields + "---\n# Plan\n" + prA + "\n"
		writeFile(t, filepath.Join(root, "github.com/acme", file.repo, "260910120000-"+file.name+".md"), content)
	}
	writeFile(t, filepath.Join(root, "legacy.md"), "# Legacy\n"+prA+"\n")
	writeFile(t, filepath.Join(root, "broken.md"), "---\nPullRequests: ["+prA+"]\nType: [broken\n---\n")
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"all statuses", nil, []string{"abandoned", "superseded", "active", "done"}},
		{"trimmed URL", []string{"--pr", " " + prA + " "}, []string{"abandoned", "superseded", "active", "done"}},
		{"repository", []string{"--repo", "widgets"}, []string{"active", "done"}},
		{"excluded repository", []string{"--repo", "!=widgets"}, []string{"abandoned", "superseded"}},
		{"status", []string{"--status", "implemented"}, []string{"done"}},
		{"all", []string{"--all"}, []string{"abandoned", "superseded", "active", "done"}},
		{"named match", []string{"done"}, []string{"done"}},
		{"named nonmatch", []string{"other-pr"}, nil},
		{"no matches", []string{"--pr", "https://github.com/acme/widgets/pull/100"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps, io := testDependencies("", true, 0)
			args := append([]string{"get", "--root", root, "--pr", prA, "-o", "name"}, tc.args...)
			if err := run(t, deps, args...); err != nil {
				t.Fatal(err)
			}
			want := ""
			for _, name := range tc.want {
				want += "260910120000-" + name + "\n"
			}
			if io.stdout.String() != want {
				t.Errorf("stdout = %q, want %q", io.stdout, want)
			}
			wantStderr := ""
			if tc.name == "no matches" {
				wantStderr = "No plans found.\n"
			}
			if io.stderr.String() != wantStderr {
				t.Errorf("stderr = %q, want %q", io.stderr, wantStderr)
			}
		})
	}
	for _, format := range []string{"", "wide", "json", "yaml"} {
		t.Run("format "+format, func(t *testing.T) {
			deps, io := testDependencies("", true, 0)
			args := []string{"get", "--root", root, "--pr", prA, "--status", "implemented"}
			if format != "" {
				args = append(args, "-o", format)
			}
			if err := run(t, deps, args...); err != nil {
				t.Fatal(err)
			}
			out := io.stdout.String()
			if !strings.Contains(out, "done") || io.stderr.Len() != 0 {
				t.Errorf("stdout = %q, stderr = %q", out, io.stderr)
			}
			if (format == "json" && !strings.Contains(out, `"items": [`)) || (format == "yaml" && !strings.HasPrefix(out, "items:\n")) {
				t.Errorf("expected a list, got %q", out)
			}
		})
	}
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "get", "--root", root, "--pr", prB, "--status", "implemented", "-o", "json"); err != nil {
		t.Fatal(err)
	}
	if got := io.stdout.String(); got != "{\n    \"items\": []\n}\n" || io.stderr.Len() != 0 {
		t.Errorf("empty json list: stdout = %q, stderr = %q", got, io.stderr)
	}
}

func TestGetPullRequestRejectsInvalidURL(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	for _, value := range []string{"", " ", "1", "acme/widgets#1", ghIssue, "https://github.com/acme/widgets", "ftp://example.com/pr/1"} {
		t.Run(value, func(t *testing.T) {
			deps, io := testDependencies("", true, 0)
			err := run(t, deps, "get", "--root", root, "--pr", value)
			if err == nil || !strings.Contains(err.Error(), "--pr:") || io.stdout.Len() != 0 {
				t.Errorf("error = %v, stdout = %q", err, io.stdout)
			}
		})
	}
}

func TestEmptyListCountsInactivePlans(t *testing.T) {
	fixtureHome(t)
	root := fixtureRoot(t)
	want := "No plans found.\n1 Implemented, Superseded, or Abandoned plan hidden (--all shows them)\n"
	for _, cmd := range []string{"get", "tree"} {
		deps, io := testDependencies("", true, 0)
		if err := run(t, deps, cmd, "--root", root, "--repo", "gizmos"); err != nil {
			t.Fatal(err)
		}
		if io.stdout.Len() != 0 || io.stderr.String() != want {
			t.Errorf("%s --repo gizmos: stdout = %q, stderr = %q", cmd, io.stdout, io.stderr)
		}
		deps, io = testDependencies("", true, 0)
		if err := run(t, deps, cmd, "--root", root, "--repo", "gizmos", "--status", "InProgress"); err != nil {
			t.Fatal(err)
		}
		if io.stdout.Len() != 0 || io.stderr.String() != "No plans found.\n" {
			t.Errorf("%s --repo gizmos --status InProgress: stdout = %q, stderr = %q", cmd, io.stdout, io.stderr)
		}
	}
}

func TestAbandonedStatus(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "github.com", "acme", "widgets", "260910120000-abandoned-plan.md")
	input := "---\nType: plan\nImplementationStatus: InProgress\nStatusChecked: 2026-09-01\nStatusNote: \"Old note.\"\n---\n\n# Abandoned plan\n\nBody.\n"
	writeFile(t, path, input)
	deps, _ := testDependencies("", true, 0)
	if err := run(t, deps, "set", "status", "--root", root, "abandoned-plan", "aBaNdOnEd", "--note", "Will not be implemented."); err != nil {
		t.Fatal(err)
	}
	want := strings.NewReplacer("InProgress", "Abandoned", "2026-09-01", "2026-09-29", "Old note.", "Will not be implemented.").Replace(input)
	if got := readFile(t, path); got != want {
		t.Errorf("content:\n%s", got)
	}
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "check", "--root", root); err != nil {
		t.Fatal(err)
	}
	if io.stdout.Len() != 0 || io.stderr.String() != "No findings.\n" {
		t.Errorf("check: stdout = %q, stderr = %q", io.stdout, io.stderr)
	}
	for _, cmd := range []string{"get", "tree"} {
		for _, tc := range []struct {
			args []string
			show bool
		}{
			{nil, false},
			{[]string{"--all"}, true},
			{[]string{"--status", "abandoned"}, true},
			{[]string{"abandoned-plan"}, true},
		} {
			deps, io := testDependencies("", true, 0)
			deps.now = func() time.Time { return fixedNow.AddDate(0, 0, 14) }
			if err := run(t, deps, append([]string{cmd, "--root", root}, tc.args...)...); err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(io.stdout.String(), "abandoned-plan"); got != tc.show {
				t.Errorf("%s %v: stdout = %q", cmd, tc.args, io.stdout)
			}
			if tc.show {
				if !strings.Contains(io.stdout.String(), "Abandoned") || !strings.Contains(io.stdout.String(), "14d") || strings.Contains(io.stdout.String(), "!") || io.stderr.Len() != 0 {
					t.Errorf("%s %v: stdout = %q, stderr = %q", cmd, tc.args, io.stdout, io.stderr)
				}
			} else if io.stderr.String() != "No plans found.\n1 Implemented, Superseded, or Abandoned plan hidden (--all shows them)\n" {
				t.Errorf("%s: stderr = %q", cmd, io.stderr)
			}
		}
	}
}

func TestStatusFilterRejectsUnknownValue(t *testing.T) {
	fixtureHome(t)
	root := fixtureRoot(t)
	for _, cmd := range []string{"get", "tree"} {
		deps, _ := testDependencies("", true, 0)
		err := run(t, deps, cmd, "--root", root, "--status", "done")
		if err == nil || !strings.Contains(err.Error(), `unknown status "done"`) {
			t.Errorf("%s --status done: error = %v", cmd, err)
		}
	}
}

func TestWideIgnoresTerminalWidth(t *testing.T) {
	fixtureHome(t)
	root := fixtureRoot(t)
	deps, io := testDependencies("", true, 40)
	if err := run(t, deps, "tree", "--root", root, "-o", "wide"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(io.stdout.String(), ellipsis) {
		t.Errorf("-o wide output was truncated:\n%s", io.stdout)
	}
}

func TestTreeFailsWhenRootIsMissing(t *testing.T) {
	deps, _ := testDependencies("", true, 0)
	missing := filepath.Join(t.TempDir(), "nope")
	err := run(t, deps, "tree", "--root", missing)
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("error = %v, want one naming %s", err, missing)
	}
}

func TestCheckGolden(t *testing.T) {
	fixtureHome(t)
	root := fixtureRoot(t)
	deps, io := testDependencies("", true, 0)
	err := run(t, deps, "check", "--root", root)
	if err == nil || err.Error() != "check: 20 errors, 9 advisories" {
		t.Fatalf("error = %v", err)
	}
	assertGolden(t, "check.golden", io.stdout.String())
}

func TestCheckNamedPlans(t *testing.T) {
	fixtureHome(t)
	root := fixtureRoot(t)
	deps, io := testDependencies("", true, 0)
	err := run(t, deps, "check", "--root", root, "widgets-links", "widgets-umbrella")
	if err == nil || err.Error() != "check: 2 errors, 2 advisories" {
		t.Fatalf("error = %v", err)
	}
	out := io.stdout.String()
	if lines := strings.Split(strings.TrimSpace(out), "\n"); len(lines) != 5 {
		t.Errorf("want a header and four rows:\n%s", out)
	}
	for _, unwanted := range []string{"widgets-bad-yaml", "widgets-cycle-a", "legacy-name.md"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("findings of an unnamed plan %q were printed:\n%s", unwanted, out)
		}
	}
	deps, io = testDependencies("", true, 0)
	if err := run(t, deps, "check", "--root", root, "widgets-child-active"); err != nil || io.stdout.Len() != 0 || io.stderr.String() != "No findings.\n" {
		t.Errorf("clean plan: error = %v, stdout = %q, stderr = %q", err, io.stdout, io.stderr)
	}
	deps, io = testDependencies("", true, 0)
	err = run(t, deps, "check", "--root", root, "nope")
	if err == nil || !strings.Contains(err.Error(), `"nope"`) || io.stdout.Len() != 0 || io.stderr.Len() != 0 {
		t.Errorf("unknown plan: error = %v, stdout = %q, stderr = %q", err, io.stdout, io.stderr)
	}
	deps, io = testDependencies("", true, 0)
	err = run(t, deps, "check", "--root", root, "widgets-links", "nope")
	if err == nil || !strings.Contains(err.Error(), `"nope"`) || !strings.Contains(err.Error(), "check: 2 errors, 1 advisory") || !strings.Contains(io.stdout.String(), "widgets-links") {
		t.Errorf("found and missing: error = %v, stdout = %q", err, io.stdout)
	}
}

func TestCheckCleanCorpusReportsNoFindings(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "github.com", "a", "b", "260101000000-ok.md"), "---\nType: plan\nImplementationStatus: Implemented\nStatusChecked: 2026-01-01\nStatusNote: \"Done.\"\n---\n# Ok\n")
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "check", "--root", root); err != nil {
		t.Fatal(err)
	}
	if io.stdout.Len() != 0 || io.stderr.String() != "No findings.\n" {
		t.Errorf("stdout = %q, stderr = %q", io.stdout, io.stderr)
	}
}

func TestDuplicateTitlesWithSuccessors(t *testing.T) {
	for _, tc := range []struct {
		name, a, b, c string
		duplicate     bool
	}{
		{"pair", "[[260910120000-b]]", "", "different", false},
		{"chain", "[[260910120000-b]]", "[[260910120000-c]]", "", false},
		{"two remaining", "[[260910120000-b]]", "", "", true},
		{"active with successor", "active", "", "different", true},
		{"unresolved", "[[missing]]", "", "different", true},
		{"self", "[[260910120000-a]]", "", "different", true},
		{"cycle", "[[260910120000-b]]", "[[260910120000-a]]", "different", true},
		{"outside group", "[[260910120000-c]]", "", "different", true},
		{"relative path", "./260910120000-b.md", "", "different", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for i, successor := range []string{tc.a, tc.b, tc.c} {
				front, title := logFront, "Shared title"
				if successor == "different" {
					title = "Different title"
				} else if successor != "" {
					if successor == "active" {
						successor = "[[260910120000-b]]"
					} else {
						front = strings.ReplaceAll(front, "InProgress", "Superseded")
					}
					front = strings.Replace(front, "Type: plan\n", "Type: plan\nSupersededBy: "+quoteYAML(successor)+"\n", 1)
				}
				writeFile(t, filepath.Join(root, "260910120000-"+string(rune('a'+i))+".md"), front+"# "+title+"\n")
			}
			deps, output := testDependencies("", true, 0)
			if err := run(t, deps, "check", "--root", root); err != nil && tc.name != "unresolved" {
				t.Fatal(err)
			}
			if got := strings.Contains(output.stdout.String(), "duplicate-title"); got != tc.duplicate {
				t.Errorf("duplicate-title = %v, want %v; output: %s", got, tc.duplicate, output.stdout)
			}
			if tc.name == "two remaining" && strings.Contains(output.stdout.String(), "260910120000-a.md") {
				t.Errorf("replaced plan appears in the finding: %s", output.stdout)
			}
		})
	}
}

func TestCheckExitStatus(t *testing.T) {
	legacy := "# Legacy\n"
	broken := "---\nType: plan\nType: plan\n---\n"
	cases := []struct {
		name    string
		files   map[string]string
		wantErr string
	}{
		{"no findings", map[string]string{"github.com/a/b/260101000000-ok.md": "---\nType: plan\nImplementationStatus: Implemented\nStatusChecked: 2026-01-01\nStatusNote: \"Done.\"\n---\n# Ok\n"}, ""},
		{"advisories only", map[string]string{"github.com/a/b/260101000000-legacy.md": legacy}, ""},
		{"errors only", map[string]string{"github.com/a/b/260101000000-broken.md": broken}, "check: 1 error, 0 advisories"},
		{"mixed", map[string]string{"github.com/a/b/260101000000-legacy.md": legacy, "github.com/a/b/260102000000-broken.md": broken}, "check: 1 error, 1 advisory"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for name, content := range tc.files {
				writeFile(t, filepath.Join(root, name), content)
			}
			deps, _ := testDependencies("", true, 0)
			err := run(t, deps, "check", "--root", root)
			got := ""
			if err != nil {
				got = err.Error()
			}
			if got != tc.wantErr {
				t.Fatalf("error = %q, want %q", got, tc.wantErr)
			}
		})
	}
}

func TestCheckHelpListsRulesAndStatuses(t *testing.T) {
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "check", "--help"); err != nil {
		t.Fatal(err)
	}
	help := io.stdout.String()
	for _, want := range append(statusValues, "no-front-matter", "duplicate-title", "invalid-link", "unknown-key", "missing-successor", "successor-not-found", "stale-pr-note", "pr-state-unknown", "--github", "advisory", "error", "FILE", "SEVERITY") {
		if !strings.Contains(help, want) {
			t.Errorf("check --help lacks %q", want)
		}
	}
}

func TestGetHelpListsRecordKeys(t *testing.T) {
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "get", "--help"); err != nil {
		t.Fatal(err)
	}
	help := io.stdout.String()
	for _, want := range []string{"name, basename, path, repo, title", "front_matter_error", "Type, Parent, Issues", "ImplementationStatus, SupersededBy, StatusChecked", "StatusNote"} {
		if !strings.Contains(help, want) {
			t.Errorf("get --help lacks %q", want)
		}
	}
}

func TestHelpNeedsNoConfiguration(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "--help"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(io.stdout.String(), "planner [command]") {
		t.Errorf("unexpected help:\n%s", io.stdout)
	}
}

func TestUnknownSubcommandFails(t *testing.T) {
	deps, _ := testDependencies("", true, 0)
	if err := run(t, deps, "list"); err == nil {
		t.Fatal("expected an error for an unknown subcommand")
	}
}

func TestBareCommandPrintsHelpWithoutConfiguration(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Available Commands", "tree", "get", "create", "set"} {
		if !strings.Contains(io.stdout.String(), want) {
			t.Errorf("help lacks %q:\n%s", want, io.stdout)
		}
	}
}

func TestResolveRootFromFlag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFile(t, filepath.Join(home, configName), "root: [broken\n")
	work := t.TempDir()
	t.Chdir(work)
	got, err := resolveRoot("plans")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(work, "plans"); got != want {
		t.Errorf("root = %q, want %q", got, want)
	}
	got, err = resolveRoot("~/elsewhere")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "elsewhere"); got != want {
		t.Errorf("root = %q, want %q", got, want)
	}
}

func TestResolveRootFromConfig(t *testing.T) {
	cases := []struct {
		name    string
		config  string
		want    func(home string) string
		wantErr error
		errText string
	}{
		{name: "missing file", wantErr: errNoRoot},
		{name: "empty root", config: "root: \"\"\n", wantErr: errNoRoot},
		{name: "other keys only", config: "editor: code\n", wantErr: errNoRoot},
		{name: "malformed", config: "root: [broken\n", errText: "parse "},
		{name: "home relative", config: "root: ~/plans\n", want: func(home string) string { return filepath.Join(home, "plans") }},
		{name: "relative to config", config: "root: notes/plans\n", want: func(home string) string { return filepath.Join(home, "notes", "plans") }},
		{name: "absolute", config: "root: /srv/plans\n", want: func(string) string { return "/srv/plans" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			if tc.config != "" {
				writeFile(t, filepath.Join(home, configName), tc.config)
			}
			t.Chdir(t.TempDir())
			got, err := resolveRoot("")
			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				if !strings.Contains(err.Error(), "root: ~/plans") || !strings.Contains(err.Error(), "--root") {
					t.Errorf("error lacks the example or the flag: %v", err)
				}
			case tc.errText != "":
				if err == nil || !strings.Contains(err.Error(), tc.errText) || !strings.Contains(err.Error(), filepath.Join(home, configName)) {
					t.Fatalf("error = %v, want one containing %q and the config path", err, tc.errText)
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
				if want := tc.want(home); got != want {
					t.Errorf("root = %q, want %q", got, want)
				}
			}
		})
	}
}

func TestNamedPlansMustResolve(t *testing.T) {
	fixtureHome(t)
	root := fixtureRoot(t)
	for _, args := range [][]string{{"get", "nope"}, {"tree", "nope"}, {"describe", "nope"}, {"get", "shared-name"}} {
		deps, io := testDependencies("", true, 0)
		err := run(t, deps, append(args, "--root", root)...)
		if err == nil || io.stdout.Len() != 0 {
			t.Errorf("%v: error = %v, stdout = %q", args, err, io.stdout)
		}
	}
}

func TestNamedPlansPrintFoundAndReportMissing(t *testing.T) {
	fixtureHome(t)
	root := fixtureRoot(t)
	for _, cmd := range []string{"get", "tree", "describe"} {
		deps, io := testDependencies("", true, 0)
		err := run(t, deps, cmd, "--root", root, "widgets-child-done", "nope", "widgets-crlf")
		if err == nil || !strings.Contains(err.Error(), `"nope"`) {
			t.Errorf("%s: error = %v", cmd, err)
		}
		out := io.stdout.String()
		if !strings.Contains(out, "widgets-child-done") || !strings.Contains(out, "widgets-crlf") {
			t.Errorf("%s did not print the plans it found:\n%s", cmd, out)
		}
		if io.stderr.Len() != 0 {
			t.Errorf("%s: stderr = %q", cmd, io.stderr)
		}
	}
	deps, io := testDependencies("", true, 0)
	err := run(t, deps, "get", "--root", root, "-o", "json", "nope")
	if err == nil || io.stdout.Len() != 0 {
		t.Errorf("get -o json nope: error = %v, stdout = %q", err, io.stdout)
	}
}

func TestOutputFormatIsValidated(t *testing.T) {
	fixtureHome(t)
	root := fixtureRoot(t)
	for _, args := range [][]string{{"get", "-o", "table"}, {"tree", "-o", "json"}, {"tree", "-o", "name"}} {
		deps, _ := testDependencies("", true, 0)
		err := run(t, deps, append(args, "--root", root)...)
		if err == nil || !strings.Contains(err.Error(), "unknown output format") {
			t.Errorf("%v: error = %v", args, err)
		}
	}
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "get", "--root", root, "-o", "json", "--repo", "nomatch"); err != nil {
		t.Fatal(err)
	}
	if got := io.stdout.String(); got != "{\n    \"items\": []\n}\n" || io.stderr.Len() != 0 {
		t.Errorf("empty json list: stdout = %q, stderr = %q", got, io.stderr)
	}
}

func TestCompletion(t *testing.T) {
	fixtureHome(t)
	root := fixtureRoot(t)
	cases := []struct {
		args     []string
		want     []string
		unwanted []string
	}{
		{[]string{"get", "--root", root, "widgets-u"}, []string{"widgets-umbrella\tWidgets umbrella", "widgets-unknown-status\t"}, []string{"gadgets-cross-child", "widgets-crlf"}},
		{[]string{"get", "--root", root, "260906"}, nil, []string{"260906120000-shared-name"}},
		{[]string{"get", "--root", root, root + "/github.com/acme/g"}, []string{root + "/github.com/acme/gadgets/260906120000-shared-name.md\t", root + "/github.com/acme/gizmos/260906120000-shared-name.md\t"}, nil},
		{[]string{"get", "--root", root, "sha"}, nil, []string{"shared-name"}},
		{[]string{"describe", "--root", root, "widgets-umbrella", "widgets-"}, []string{"widgets-crlf\t"}, []string{"widgets-umbrella\t"}},
		{[]string{"set", "status", "--root", root, "widgets-umbrella", "In"}, []string{"InProgress"}, []string{"Implemented", "widgets-"}},
		{[]string{"set", "status", "--root", root, "widgets-umbrella", "Ab"}, []string{"Abandoned"}, []string{"Superseded"}},
		{[]string{"get", "--root", root, "--status", "Ab"}, []string{"Abandoned"}, []string{"Superseded"}},
		{[]string{"tree", "--root", root, "--status", "Ab"}, []string{"Abandoned"}, []string{"Superseded"}},
		{[]string{"create", "--root", root, "--status", "Ab"}, []string{"Abandoned"}, []string{"Superseded"}},
		{[]string{"get", "--root", root, "--repo", "acme/g"}, []string{"acme/gadgets", "acme/gizmos"}, []string{"acme/widgets"}},
		{[]string{"get", "--root", root, "--repo", "!=acme/g"}, []string{"!=acme/gadgets", "!=acme/gizmos"}, []string{"acme/widgets"}},
		{[]string{"get", "--root", root, "rep"}, []string{"repos\tList every repository that has plans"}, []string{"widgets-"}},
		{[]string{"tree", "--root", root, "--status", "Sup"}, []string{"Superseded"}, []string{"InProgress"}},
		{[]string{"get", "--root", root, "-o", ""}, []string{"wide", "json", "yaml", "name"}, nil},
		{[]string{"tree", "--root", root, "-o", ""}, []string{"wide"}, []string{"json"}},
		{[]string{"create", "--root", root, "--status", "Sup"}, []string{"Superseded"}, []string{"InProgress"}},
		{[]string{"create", "--root", root, "--parent", "widgets-um"}, []string{"widgets-umbrella\t"}, nil},
		{[]string{"set", "parent", "--root", root, "widgets-umbrella", "widgets-u"}, []string{"widgets-unknown-status\t"}, []string{"widgets-umbrella\t"}},
		{[]string{"check", "--root", root, "widgets-um"}, []string{"widgets-umbrella\tWidgets umbrella"}, nil},
		{[]string{"log", "--root", root, "widgets-um"}, []string{"widgets-umbrella\tWidgets umbrella"}, nil},
		{[]string{"log", "--root", root, "widgets-umbrella", "widgets-"}, nil, []string{"widgets-crlf"}},
	}
	for _, tc := range cases {
		deps, io := testDependencies("", true, 0)
		if err := run(t, deps, append([]string{"__complete"}, tc.args...)...); err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		out := io.stdout.String()
		for _, want := range tc.want {
			if !strings.Contains(out, want) {
				t.Errorf("%v: completions lack %q:\n%s", tc.args, want, out)
			}
		}
		for _, unwanted := range tc.unwanted {
			if strings.Contains(out, unwanted) {
				t.Errorf("%v: completions include %q:\n%s", tc.args, unwanted, out)
			}
		}
	}
	t.Setenv("HOME", t.TempDir())
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "__complete", "get", ""); err != nil {
		t.Fatal(err)
	}
	got := io.stdout.String()
	if rest, ok := strings.CutPrefix(got, "repos\tList every repository that has plans\n"); !ok || !strings.HasPrefix(rest, ":") {
		t.Errorf("completion without a root printed %q", got)
	}
}

func TestPlanCompletionsResolve(t *testing.T) {
	fixtureHome(t)
	root := fixtureRoot(t)
	c, err := loadCorpus(root)
	if err != nil {
		t.Fatal(err)
	}
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "__complete", "get", "--root", root, ""); err != nil {
		t.Fatal(err)
	}
	getSubcommands := map[string]bool{"repos": true}
	for _, line := range strings.Split(strings.TrimSpace(io.stdout.String()), "\n") {
		if strings.HasPrefix(line, ":") {
			continue
		}
		ref, _, _ := strings.Cut(line, "\t")
		if getSubcommands[ref] {
			continue
		}
		if _, err := findPlan(c, ref); err != nil {
			t.Errorf("completion %q does not resolve: %v", ref, err)
		}
	}
}
