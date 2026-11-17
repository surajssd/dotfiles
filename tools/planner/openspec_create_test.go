package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func fakeOpenSpec(t *testing.T, root string) (string, string) {
	t.Helper()
	bin := t.TempDir()
	script := readFile(t, "testdata/fake-openspec.sh")
	if err := os.WriteFile(filepath.Join(bin, "openspec"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	calls := filepath.Join(bin, "calls")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OPENSPEC_CALLS", calls)
	t.Setenv("OPENSPEC_TEST_MODE", "")
	store := filepath.Join(root, "github.com/acme/widgets")
	writeFile(t, filepath.Join(store, "openspec/config.yaml"), "schema: spec-driven\n")
	if err := os.MkdirAll(filepath.Join(store, "openspec/changes"), 0o755); err != nil {
		t.Fatal(err)
	}
	return store, calls
}

func TestCreateUsesOpenSpecByDefault(t *testing.T) {
	root := filepath.Join(t.TempDir(), "plans with spaces")
	store, calls := fakeOpenSpec(t, root)
	initRepo(t, "upstream", "https://github.com/acme/widgets")
	parent := parentFixture(t, root)
	input := strings.Replace(generatedHead, "Type: plan", "ID: 260929120000-add-feature\nType: plan\nCustom: keep", 1) + "# My plan\n"
	deps, output := testDependencies(strings.ReplaceAll(input, "\n", "\r\n"), false, 0)
	if err := run(t, deps, "create", "--root", root, "add", "feature", "--parent", parent, "--pr", "https://github.com/acme/widgets/pull/3", "--status", "in-progress", "--note", "Started.", "--issue", "https://example.com/issue"); err != nil {
		t.Fatal(err)
	}
	metadata := filepath.Join(store, "openspec/changes/add-feature/plan.md")
	resolved, err := filepath.EvalSymlinks(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if output.stdout.String() != resolved+"\n" {
		t.Fatalf("stdout: %s", output.stdout)
	}
	content := readFile(t, metadata)
	for _, want := range []string{"ID: 260929120000-add-feature", "ImplementationStatus: InProgress", "Started.", "https://example.com/issue", "# My plan\r\n", "Custom: keep\r\n", "Parent: \"[[260901000000-parent-plan]]\"", "https://github.com/acme/widgets/pull/3"} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q: %s", want, content)
		}
	}
	if got := readFile(t, calls); got != "list\x00--json\x00new\x00change\x00add-feature\x00--json\x00" {
		t.Fatalf("CLI arguments: %q", got)
	}
	if readFile(t, filepath.Join(filepath.Dir(metadata), "proposal.md")) != "# Preserve\n" {
		t.Error("artifact changed")
	}
}

func TestOpenSpecCreationValidatesBeforeMutation(t *testing.T) {
	for _, scenario := range []string{"wrong-root", "existing", "input", "conflicting-id", "collision", "invalid-name", "bad-parent", "duplicate-key", "invalid-yaml", "symlink-store", "managed-ancestor"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			store, calls := fakeOpenSpec(t, root)
			initRepo(t, "upstream", "https://github.com/acme/widgets")
			deps, output := testDependencies("", true, 0)
			name := "feature"
			switch scenario {
			case "wrong-root":
				t.Setenv("OPENSPEC_TEST_MODE", scenario)
			case "existing":
				writeFile(t, filepath.Join(store, "openspec/changes/feature/proposal.md"), "original")
			case "input":
				deps.stdinIsTerminal = func() bool { return false }
				deps.stdin = errReader{}
			case "conflicting-id":
				deps, output = testDependencies("---\nID: 260929120000-other\n---\n", false, 0)
			case "collision":
				directoryFixture(t, root, "other", "260929120000-feature")
			case "invalid-name":
				name = "Upper_Case"
			case "symlink-store":
				changes := filepath.Join(store, "openspec/changes")
				if err := os.Remove(changes); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(t.TempDir(), changes); err != nil {
					t.Fatal(err)
				}
			case "managed-ancestor":
				writeFile(t, filepath.Join(store, "plan.md"), "---\nID: 260929120000-ancestor\n"+strings.TrimPrefix(generatedHead, "---\n"))
			case "duplicate-key":
				deps, output = testDependencies("---\nID: 260929120000-feature\nID: 260929120000-feature\n---\n", false, 0)
			case "invalid-yaml":
				deps, output = testDependencies("---\nType: [broken\n---\n", false, 0)
			}
			args := []string{"create", "--root", root, name}
			if scenario == "bad-parent" {
				args = append(args, "--parent", "absent")
			}
			if err := run(t, deps, args...); err == nil {
				t.Fatal("creation succeeded")
			}
			if output.stdout.Len() != 0 {
				t.Errorf("stdout: %s", output.stdout)
			}
			if data, err := os.ReadFile(calls); err == nil && strings.Contains(string(data), "new\x00") {
				t.Errorf("mutation invoked: %q", data)
			}
			if _, err := os.Stat(filepath.Join(store, "openspec/changes/feature/plan.md")); !os.IsNotExist(err) {
				t.Errorf("metadata created: %v", err)
			}
		})
	}
}

func TestOpenSpecCreationFailurePreservesChange(t *testing.T) {
	for _, mode := range []string{"failure", "invalid-json", "metadata-exists", "wrong-path"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			store, calls := fakeOpenSpec(t, root)
			initRepo(t, "upstream", "https://github.com/acme/widgets")
			t.Setenv("OPENSPEC_TEST_MODE", mode)
			deps, output := testDependencies("", true, 0)
			err := run(t, deps, "create", "--root", root, "feature")
			if err == nil || !strings.Contains(err.Error(), "may exist") {
				t.Fatalf("error: %v", err)
			}
			if output.stdout.Len() != 0 {
				t.Errorf("stdout: %s", output.stdout)
			}
			artifact := filepath.Join(store, "openspec/changes/feature/proposal.md")
			if readFile(t, artifact) != "# Preserve\n" {
				t.Error("artifact changed")
			}
			if mode == "metadata-exists" && readFile(t, filepath.Join(filepath.Dir(artifact), "plan.md")) != "original" {
				t.Error("existing metadata changed")
			}
			if strings.Count(readFile(t, calls), "new\x00") != 1 {
				t.Error("mutation retried")
			}
		})
	}
}

func TestOpenSpecDependencyAndOptOut(t *testing.T) {
	root := t.TempDir()
	initRepo(t, "upstream", "https://github.com/acme/widgets")
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.Symlink(git, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	deps, output := testDependencies("", true, 0)
	if err := run(t, deps, "create", "--root", root, "feature"); err == nil || !strings.Contains(err.Error(), "--no-openspec") {
		t.Fatalf("missing CLI: %v", err)
	}
	if len(listFiles(t, root)) != 0 || output.stdout.Len() != 0 {
		t.Fatal("missing CLI created a plan")
	}
	if err := run(t, deps, "create", "--root", root, "--no-openspec", "feature"); err != nil {
		t.Fatal(err)
	}
	if err := run(t, deps, "create", "--root", root, "--in", root, "feature"); err == nil {
		t.Fatal("removed --in flag accepted")
	}
}

func TestOpenSpecCommandCancellation(t *testing.T) {
	root := t.TempDir()
	store, _ := fakeOpenSpec(t, root)
	binary, err := exec.LookPath("openspec")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := invokeOpenSpec(ctx, binary, store, "list", "--json"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestOpenSpecMetadataWriteFailure(t *testing.T) {
	if root := os.Getenv("PLANNER_TEST_OPENSPEC_WRITE_ROOT"); root != "" {
		signal.Ignore(syscall.SIGXFSZ)
		if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &syscall.Rlimit{Cur: 32, Max: 32}); err != nil {
			t.Fatal(err)
		}
		deps, _ := testDependencies("", true, 0)
		if err := run(t, deps, "create", "--root", root, "feature"); err == nil || !strings.Contains(err.Error(), "file too large") {
			t.Fatalf("write error: %v", err)
		}
		return
	}
	root := t.TempDir()
	store, _ := fakeOpenSpec(t, root)
	initRepo(t, "upstream", "https://github.com/acme/widgets")
	cmd := exec.Command(os.Args[0], "-test.run=^TestOpenSpecMetadataWriteFailure$")
	cmd.Env = append(os.Environ(), "PLANNER_TEST_OPENSPEC_WRITE_ROOT="+root, "OPENSPEC_CALLS=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("write exercise: %v\n%s", err, out)
	}
	change := filepath.Join(store, "openspec/changes/feature")
	if _, err := os.Stat(filepath.Join(change, "plan.md")); !os.IsNotExist(err) {
		t.Errorf("metadata remains: %v", err)
	}
	if readFile(t, filepath.Join(change, "proposal.md")) != "# Preserve\n" {
		t.Error("artifact changed")
	}
}

func TestOpenSpecInitializesExternalTree(t *testing.T) {
	for _, layout := range []string{"existing-plans", "missing-repository", "missing-root", "incomplete-tree", "symlinked-root"} {
		t.Run(layout, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "plans with spaces")
			store, calls := fakeOpenSpec(t, root)
			legacy := filepath.Join(store, "260901000000-existing.md")
			writeFile(t, legacy, generatedHead+"# Existing plan\n")
			if layout == "incomplete-tree" {
				if err := os.Remove(filepath.Join(store, "openspec/changes")); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.RemoveAll(filepath.Join(store, "openspec")); err != nil {
					t.Fatal(err)
				}
			}
			if layout == "missing-repository" {
				if err := os.RemoveAll(store); err != nil {
					t.Fatal(err)
				}
			}
			if layout == "missing-root" {
				if err := os.RemoveAll(root); err != nil {
					t.Fatal(err)
				}
			}
			if layout == "symlinked-root" {
				link := filepath.Join(t.TempDir(), "linked-root")
				if err := os.Symlink(root, link); err != nil {
					t.Fatal(err)
				}
				root = link
			}
			initRepo(t, "upstream", "https://github.com/acme/widgets")
			deps, output := testDependencies("# First change\n", false, 0)
			if err := run(t, deps, "create", "--root", root, "feature"); err != nil {
				t.Fatal(err)
			}
			metadata, err := filepath.EvalSymlinks(filepath.Join(store, "openspec/changes/feature/plan.md"))
			if err != nil {
				t.Fatal(err)
			}
			if output.stdout.String() != metadata+"\n" {
				t.Fatalf("stdout: %q", output.stdout)
			}
			if readFile(t, calls) != "init\x00--tools\x00none\x00--no-animation\x00list\x00--json\x00new\x00change\x00feature\x00--json\x00" {
				t.Fatalf("CLI calls: %q", readFile(t, calls))
			}
			if layout != "missing-root" && layout != "missing-repository" && readFile(t, legacy) != generatedHead+"# Existing plan\n" {
				t.Error("existing plan changed")
			}
			if layout == "incomplete-tree" && readFile(t, filepath.Join(store, "openspec/config.yaml")) != "schema: spec-driven\n" {
				t.Error("existing config changed")
			}
		})
	}
}

func TestOpenSpecInitFailureStopsCreation(t *testing.T) {
	root := t.TempDir()
	store, calls := fakeOpenSpec(t, root)
	if err := os.RemoveAll(filepath.Join(store, "openspec")); err != nil {
		t.Fatal(err)
	}
	initRepo(t, "upstream", "https://github.com/acme/widgets")
	t.Setenv("OPENSPEC_TEST_MODE", "init-failure")
	deps, output := testDependencies("", true, 0)
	err := run(t, deps, "create", "--root", root, "feature")
	if err == nil || !strings.Contains(err.Error(), "initialization failed") {
		t.Fatalf("error: %v", err)
	}
	if output.stdout.Len() != 0 {
		t.Errorf("stdout: %s", output.stdout)
	}
	if strings.Contains(readFile(t, calls), "new\x00") {
		t.Error("created change after failed init")
	}
	if readFile(t, filepath.Join(store, "openspec/config.yaml")) != "schema: spec-driven\n" {
		t.Error("partial init config changed")
	}
}

func TestInvalidInputDoesNotInitializeOpenSpec(t *testing.T) {
	root := t.TempDir()
	store, calls := fakeOpenSpec(t, root)
	if err := os.RemoveAll(filepath.Join(store, "openspec")); err != nil {
		t.Fatal(err)
	}
	initRepo(t, "upstream", "https://github.com/acme/widgets")
	deps, _ := testDependencies("---\nID: conflicting\n---\n", false, 0)
	if err := run(t, deps, "create", "--root", root, "feature"); err == nil {
		t.Fatal("accepted conflicting ID")
	}
	if _, err := os.Stat(calls); !os.IsNotExist(err) {
		t.Errorf("invoked OpenSpec: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store, "openspec")); !os.IsNotExist(err) {
		t.Errorf("initialized invalid plan: %v", err)
	}
}
