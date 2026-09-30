package main

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("set", false, "rewrite golden files")

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
	home := t.TempDir()
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
	fixtureHome(t)
	root := fixtureRoot(t)
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
		{"tree-narrow.golden", 130, []string{"tree"}, hiddenTree},
		{"get-active.golden", 0, []string{"get"}, hiddenGet},
		{"get-all-wide.golden", 0, []string{"get", "--all", "-o", "wide"}, ""},
		{"get-json-single.golden", 0, []string{"get", "-o", "json", "widgets-umbrella"}, ""},
		{"get-json-list.golden", 0, []string{"get", "-o", "json", "--repo", "gadgets", "--all"}, ""},
		{"get-yaml.golden", 0, []string{"get", "-o", "yaml", "widgets-umbrella", "widgets-legacy"}, ""},
		{"get-name.golden", 0, []string{"get", "-o", "name", "--all"}, ""},
		{"get-no-headers.golden", 0, []string{"get", "--no-headers"}, hiddenGet},
		{"tree-no-headers.golden", 0, []string{"tree", "--no-headers", "-o", "wide", "widgets-umbrella"}, ""},
		{"get-repo.golden", 0, []string{"get", "--repo", "GADGETS", "--all"}, ""},
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
			got := strings.ReplaceAll(io.stdout.String(), root, "<root>")
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

func TestCheckCleanCorpusReportsNoFindings(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "github.com", "a", "b", "260101000000-ok.md"), "---\ntype: plan\nimplementation_status: Implemented\nstatus_checked: 2026-01-01\nstatus_note: \"Done.\"\n---\n# Ok\n")
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "check", "--root", root); err != nil {
		t.Fatal(err)
	}
	if io.stdout.Len() != 0 || io.stderr.String() != "No findings.\n" {
		t.Errorf("stdout = %q, stderr = %q", io.stdout, io.stderr)
	}
}

func TestCheckExitStatus(t *testing.T) {
	legacy := "# Legacy\n"
	broken := "---\ntype: plan\ntype: plan\n---\n"
	cases := []struct {
		name    string
		files   map[string]string
		wantErr string
	}{
		{"no findings", map[string]string{"github.com/a/b/260101000000-ok.md": "---\ntype: plan\nimplementation_status: Implemented\nstatus_checked: 2026-01-01\nstatus_note: \"Done.\"\n---\n# Ok\n"}, ""},
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
	for _, want := range append(statusValues, "no-front-matter", "duplicate-title", "invalid-link", "unknown-key", "missing-successor", "successor-not-found", "advisory", "error", "FILE", "SEVERITY") {
		if !strings.Contains(help, want) {
			t.Errorf("check --help lacks %q", want)
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
		{[]string{"get", "--root", root, "260906"}, []string{"260906120000-shared-name\tShared name in gadgets", "260906120000-shared-name\tShared name in gizmos"}, []string{"\nshared-name\t"}},
		{[]string{"get", "--root", root, "sha"}, nil, []string{"shared-name"}},
		{[]string{"describe", "--root", root, "widgets-umbrella", "widgets-"}, []string{"widgets-crlf\t"}, []string{"widgets-umbrella\t"}},
		{[]string{"set", "status", "--root", root, "widgets-umbrella", "In"}, []string{"InProgress"}, []string{"Implemented", "widgets-"}},
		{[]string{"get", "--root", root, "--repo", "acme/g"}, []string{"acme/gadgets", "acme/gizmos"}, []string{"acme/widgets"}},
		{[]string{"get", "--root", root, "-o", ""}, []string{"wide", "json", "yaml", "name"}, nil},
		{[]string{"tree", "--root", root, "-o", ""}, []string{"wide"}, []string{"json"}},
		{[]string{"create", "--root", root, "--status", "Sup"}, []string{"Superseded"}, []string{"InProgress"}},
		{[]string{"create", "--root", root, "--parent", "widgets-um"}, []string{"widgets-umbrella\t"}, nil},
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
	if got := io.stdout.String(); !strings.HasPrefix(got, ":") {
		t.Errorf("completion without a root printed %q", got)
	}
}
