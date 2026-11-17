package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func directoryFixture(t *testing.T, root, folder, id string) string {
	t.Helper()
	metadata := filepath.Join(root, "github.com/acme/widgets", folder, "plan.md")
	writeFile(t, metadata, strings.Replace(generatedHead, "Type: plan", "ID: "+id+"\nType: plan", 1)+"# Directory plan\n")
	writeFile(t, filepath.Join(filepath.Dir(metadata), "proposal.md"), "# Artifact\n[[260101000000-missing]]\n")
	writeFile(t, filepath.Join(filepath.Dir(metadata), "specs/nested/spec.md"), "# Specification\n")
	resolved, err := filepath.EvalSymlinks(metadata)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestDirectoryCommands(t *testing.T) {
	root := t.TempDir()
	id := "260929120000-directory"
	metadata := directoryFixture(t, root, "change with spaces", id)
	dir := filepath.Dir(metadata)
	parent := parentFixture(t, root)
	argsPath := fakeCode(t)
	t.Chdir(root)
	refs := []string{id, "directory", "[[" + id + "]]", metadata, dir, "./github.com/acme/widgets/change with spaces"}
	for _, ref := range refs {
		deps, output := testDependencies("", true, 0)
		if err := run(t, deps, "get", "--root", root, ref, "-o", "name"); err != nil || output.stdout.String() != id+"\n" {
			t.Fatalf("get %q: %v, %s", ref, err, output.stdout)
		}
	}
	commands := [][]string{
		{"set", "parent", dir, parent},
		{"set", "issue", id, "https://example.com/issue"},
		{"set", "pr", id, "https://github.com/acme/widgets/pull/1"},
		{"set", "status", id, "InProgress", "--note", "Started."},
		{"log", id, "Progress"},
		{"open", id},
		{"check", id},
	}
	original := strings.ReplaceAll(readFile(t, metadata), "\n", "\r\n")
	writeFile(t, metadata, original)
	if err := os.Chmod(metadata, 0o640); err != nil {
		t.Fatal(err)
	}
	for _, args := range commands {
		deps, _ := testDependencies("Entry.\n", false, 0)
		if err := run(t, deps, append(args, "--root", root)...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	content := readFile(t, metadata)
	for _, want := range []string{"ID: " + id + "\r\n", "Parent: \"[[260901000000-parent-plan]]\"", "### 2026-09-29: Progress", "Entry.", "# Directory plan\r\n"} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in %q", want, content)
		}
	}
	if strings.Contains(strings.ReplaceAll(content, "\r\n", ""), "\n") {
		t.Error("line endings changed")
	}
	info, err := os.Stat(metadata)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("permissions: %v, %v", info, err)
	}
	if got := readFile(t, argsPath); got != dir+"\x00" {
		t.Errorf("editor arguments: %q", got)
	}
	if got := readFile(t, filepath.Join(dir, "proposal.md")); got != "# Artifact\n[[260101000000-missing]]\n" {
		t.Error("artifact changed")
	}
	for _, args := range [][]string{
		{"set", "parent", id, dir},
		{"set", "status", id, "Superseded", "--superseded-by", dir},
		{"set", "parent", parent, id},
	} {
		deps, _ := testDependencies("", true, 0)
		if err := run(t, deps, append(args, "--root", root)...); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
	for _, format := range []string{"json", "yaml"} {
		deps, output := testDependencies("", true, 0)
		if err := run(t, deps, "get", "--root", root, id, "-o", format); err != nil {
			t.Fatal(err)
		}
		var record planRecord
		if err := yaml.Unmarshal(output.stdout.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		if record.ID != id || record.Directory != dir || record.Path != metadata || record.Basename != "plan" || record.Repo != "acme/widgets" {
			t.Errorf("%s record: %+v", format, record)
		}
	}
	for _, args := range [][]string{{"get", "--all", "-o", "name"}, {"tree", "--all"}, {"describe", id}, {"__complete", "get", "direc"}} {
		deps, output := testDependencies("", true, 0)
		if err := run(t, deps, append(args[:1], append([]string{"--root", root}, args[1:]...)...)...); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(output.stdout.String(), "Artifact") || !strings.Contains(output.stdout.String(), "directory") {
			t.Errorf("%v: %s", args, output.stdout)
		}
	}
}

func TestDirectoryInvalidAndDuplicateIdentity(t *testing.T) {
	for _, value := range []string{"bad", "[broken", "260929120000-duplicate"} {
		t.Run(value, func(t *testing.T) {
			root := t.TempDir()
			metadata := directoryFixture(t, root, "managed", value)
			if strings.Contains(value, "duplicate") {
				writeFile(t, filepath.Join(root, value+".md"), generatedHead+"# Legacy\n")
			}
			before := readFile(t, metadata)
			deps, output := testDependencies("", true, 0)
			err := run(t, deps, "check", "--root", root)
			if err == nil || !strings.Contains(output.stdout.String(), "error") {
				t.Fatalf("check: %v, %s", err, output.stdout)
			}
			if strings.Contains(output.stdout.String(), "no-front-matter") {
				t.Fatalf("artifact discovered: %s", output.stdout)
			}
			if strings.Contains(value, "duplicate") {
				deps, _ = testDependencies("", true, 0)
				err := run(t, deps, "set", "status", "--root", root, value, "Implemented", "--note", "Done.")
				if err == nil || !strings.Contains(err.Error(), "ambiguous") {
					t.Fatalf("duplicate mutation: %v", err)
				}
				if readFile(t, metadata) != before {
					t.Error("ambiguous mutation changed metadata")
				}
			}
		})
	}
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "plan.md"), generatedHead)
	writeFile(t, filepath.Join(root, "sibling.md"), generatedHead)
	deps, output := testDependencies("", true, 0)
	if err := run(t, deps, "get", "--root", root, "-o", "name"); err != nil || output.stdout.String() != "plan\nsibling\n" {
		t.Fatalf("legacy plan.md: %v, %s", err, output.stdout)
	}
}

func TestManagedRootExcludesArtifacts(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "plan.md"), "---\nID: 260929120000-root\n"+strings.TrimPrefix(generatedHead, "---\n"))
	writeFile(t, filepath.Join(root, "proposal.md"), "# Artifact\n")
	deps, output := testDependencies("", true, 0)
	if err := run(t, deps, "get", "--root", root, "--all", "-o", "name"); err != nil || output.stdout.String() != "260929120000-root\n" {
		t.Fatalf("managed root: %v, %s", err, output.stdout)
	}
}

func TestMalformedDirectoryMappingExcludesArtifacts(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "change/plan.md"), "---\n{ID: first, ID: second}\n---\n")
	writeFile(t, filepath.Join(root, "change/proposal.md"), "# Artifact\n")
	deps, output := testDependencies("", true, 0)
	if err := run(t, deps, "check", "--root", root); err == nil || strings.Contains(output.stdout.String(), "proposal.md") {
		t.Fatalf("malformed mapping: %v, %s", err, output.stdout)
	}
}
