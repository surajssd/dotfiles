package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenSpecDiscoveryAndArchive(t *testing.T) {
	root := t.TempDir()
	id := "260929120000-change"
	metadata := directoryFixture(t, root, "openspec/changes/change", id)
	store := filepath.Dir(filepath.Dir(filepath.Dir(metadata)))
	for _, artifact := range []string{"plan.md", "specs/capability/spec.md", "schemas/template.md", "changes/unregistered/proposal.md", "changes/unregistered/planless/tasks.md", "changes/archive/plan.md"} {
		writeFile(t, filepath.Join(store, artifact), "---\nID: 260929120000-ignored\n---\n# Ignored\n[[260101000000-missing]]\n")
	}
	parent := parentFixture(t, root)
	deps, _ := testDependencies("", true, 0)
	if err := run(t, deps, "set", "parent", "--root", root, parent, id); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(store, "changes/archive/2026-09-29-change")
	if err := os.MkdirAll(filepath.Dir(archive), 0o755); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, metadata)
	if err := os.Rename(filepath.Dir(metadata), archive); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"get", "--all", "-o", "name"}, {"check"}, {"tree"}, {"get", id, "-o", "wide"}} {
		deps, output := testDependencies("", true, 0)
		if err := run(t, deps, append(args, "--root", root)...); err != nil {
			t.Fatalf("%v: %v, %s", args, err, output.stdout)
		}
		if strings.Contains(output.stdout.String(), "Ignored") || strings.Contains(output.stdout.String(), "openspec/") && args[len(args)-1] != "wide" {
			t.Fatalf("artifacts discovered: %s", output.stdout)
		}
		if args[0] == "get" && !strings.Contains(output.stdout.String(), id) && !strings.Contains(output.stdout.String(), "acme/widgets") {
			t.Errorf("missing plan: %s", output.stdout)
		}
	}
	if readFile(t, filepath.Join(archive, "plan.md")) != before {
		t.Error("archive changed metadata")
	}
	deps, output := testDependencies("", true, 0)
	if err := run(t, deps, "get", "--root", root, "--repo", "acme/widgets", "--status", "NotImplemented", "-o", "name"); err != nil || output.stdout.String() != id+"\n" {
		t.Fatalf("status after archive: %v, %s", err, output.stdout)
	}
	deps, _ = testDependencies("", true, 0)
	if err := run(t, deps, "open", "--root", root, metadata); err == nil {
		t.Error("old path still resolved")
	}
}

func TestOpenSpecMalformedTracking(t *testing.T) {
	for _, content := range []string{"# Missing metadata\n", generatedHead, "---\nID: [broken\n---\n", "---\nID: invalid\n---\n"} {
		t.Run(content, func(t *testing.T) {
			root := t.TempDir()
			metadata := filepath.Join(root, "github.com/acme/widgets/openspec/changes/change/plan.md")
			writeFile(t, metadata, content)
			writeFile(t, filepath.Join(filepath.Dir(metadata), "tasks.md"), "# Ignored\n")
			deps, output := testDependencies("", true, 0)
			if err := run(t, deps, "check", "--root", root); err == nil || !strings.Contains(output.stdout.String(), "invalid-id") {
				t.Fatalf("check: %v, %s", err, output.stdout)
			}
			if strings.Contains(output.stdout.String(), "tasks.md") {
				t.Fatal("task artifact discovered")
			}
		})
	}
}
