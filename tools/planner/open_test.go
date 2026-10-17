package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fakeCodeScript = `#!/usr/bin/env bash
set -euo pipefail
printf '%s\0' "$@" >"$PLANNER_CODE_ARGS"
if [[ "${PLANNER_CODE_FAIL:-}" == "1" ]]; then
    printf '%s\n' 'VS Code launch failed' >&2
    exit 7
fi
`

func fakeCode(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "code"), []byte(fakeCodeScript), 0o755); err != nil {
		t.Fatal(err)
	}
	argsPath := filepath.Join(bin, "args")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PLANNER_CODE_ARGS", argsPath)
	t.Setenv("PLANNER_CODE_FAIL", "")
	return argsPath
}

func TestOpenPlan(t *testing.T) {
	home := fixtureHome(t)
	root := filepath.Join(home, "plans with spaces")
	planPath := filepath.Join(root, "github.com/acme/widgets/260910120000-edit-plan.md")
	url := "https://github.com/acme/widgets/pull/1"
	writeFile(t, planPath, "---\nType: plan\nImplementationStatus: Implemented\nPullRequests: ["+url+"]\n---\n# Edit plan\n")
	writeFile(t, filepath.Join(root, "legacy.md"), "# Legacy plan\n")
	writeFile(t, filepath.Join(root, "broken.md"), "---\nType: [broken\n---\n# Broken plan\n")
	writeFile(t, filepath.Join(home, configName), "root: ~/plans with spaces\n")
	t.Chdir(home)
	argsPath := fakeCode(t)
	cases := []struct{ ref, path string }{
		{"edit-plan", planPath},
		{"260910120000-edit-plan", planPath},
		{"260910120000-edit-plan.md", planPath},
		{"[[260910120000-edit-plan|Edit plan]]", planPath},
		{planPath, planPath},
		{"./plans with spaces/github.com/acme/widgets/260910120000-edit-plan.md", planPath},
		{"~/plans with spaces/github.com/acme/widgets/260910120000-edit-plan.md", planPath},
		{url, planPath},
		{"legacy", filepath.Join(root, "legacy.md")},
		{"broken", filepath.Join(root, "broken.md")},
	}
	for _, tc := range cases {
		t.Run(tc.ref, func(t *testing.T) {
			before := readFile(t, tc.path)
			deps, io := testDependencies("", true, 0)
			if err := run(t, deps, "open", tc.ref); err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, argsPath); got != tc.path+"\x00" {
				t.Errorf("code arguments = %q, want %q", got, tc.path+"\x00")
			}
			if io.stdout.Len() != 0 || io.stderr.Len() != 0 {
				t.Errorf("stdout = %q, stderr = %q", io.stdout, io.stderr)
			}
			if got := readFile(t, tc.path); got != before {
				t.Errorf("opening the plan changed its contents: %q", got)
			}
		})
	}
}

func TestOpenRequiresOneMatchingPlan(t *testing.T) {
	fixtureHome(t)
	root := fixtureRoot(t)
	argsPath := fakeCode(t)
	cases := []struct {
		args []string
		want string
	}{
		{nil, "accepts 1 arg(s), received 0"},
		{[]string{"widgets-umbrella", "widgets-crlf"}, "accepts 1 arg(s), received 2"},
		{[]string{"does-not-exist"}, "plan not found"},
		{[]string{"shared-name"}, "ambiguous"},
		{[]string{"https://acme.atlassian.net/browse/WID-1"}, "ambiguous"},
	}
	for _, tc := range cases {
		deps, io := testDependencies("", true, 0)
		err := run(t, deps, append([]string{"open", "--root", root}, tc.args...)...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("open %v: error = %v, want %q", tc.args, err, tc.want)
		}
		if io.stdout.Len() != 0 || io.stderr.Len() != 0 {
			t.Errorf("open %v: stdout = %q, stderr = %q", tc.args, io.stdout, io.stderr)
		}
	}
	if _, err := os.Stat(argsPath); !os.IsNotExist(err) {
		t.Fatalf("code was invoked for an invalid reference: %v", err)
	}
}

func TestOpenEditorErrors(t *testing.T) {
	fixtureHome(t)
	root := fixtureRoot(t)
	t.Run("missing code", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		deps, _ := testDependencies("", true, 0)
		err := run(t, deps, "open", "--root", root, "widgets-umbrella")
		if err == nil || !strings.Contains(err.Error(), `VS Code command "code" not found on PATH`) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("code fails", func(t *testing.T) {
		fakeCode(t)
		t.Setenv("PLANNER_CODE_FAIL", "1")
		deps, io := testDependencies("", true, 0)
		err := run(t, deps, "open", "--root", root, "widgets-umbrella")
		if err == nil || !strings.Contains(err.Error(), "exit status 7") {
			t.Fatalf("error = %v", err)
		}
		if io.stderr.String() != "VS Code launch failed\n" || io.stdout.Len() != 0 {
			t.Errorf("stdout = %q, stderr = %q", io.stdout, io.stderr)
		}
	})
}

func TestOpenHelpWithoutConfiguration(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	deps, io := testDependencies("", true, 0)
	if err := run(t, deps, "open", "--help"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"open <plan>", "VS Code", "PATH", "exactly one plan"} {
		if !strings.Contains(io.stdout.String(), want) {
			t.Errorf("help lacks %q:\n%s", want, io.stdout)
		}
	}
}
