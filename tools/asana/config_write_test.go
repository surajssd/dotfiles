package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assertConfigText(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != want {
		t.Fatalf("config = %q, error = %v, want %q", data, err, want)
	}
}

func assertNoConfigTemps(t *testing.T, path string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".asana-*.yaml"))
	if err != nil || len(files) != 0 {
		t.Fatalf("temporary files = %v, error = %v", files, err)
	}
}

func TestProjectConfigPreservation(t *testing.T) {
	f := newFixture(t, projectHandler(t, nil))
	original := "# Credentials\ntoken_command: [echo, unused] # Keep command\n# Default selection\ndefault_project: work\nprojects: # Aliases\n  work: 'https://app.asana.com/1/100/project/200' # Work URL\n"
	f.configure(t, original)
	target := filepath.Join(filepath.Dir(f.configPath), "target.yaml")
	if err := os.Rename(f.configPath, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.yaml", f.configPath); err != nil {
		t.Fatal(err)
	}
	if code := f.execute("add", "project", "Research", "--alias", "# next: 世界"); code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, &f.stderr)
	}
	link, err := os.Readlink(f.configPath)
	if err != nil || link != "target.yaml" {
		t.Fatalf("symlink = %q, error = %v", link, err)
	}
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("stat = %v, error = %v", info, err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), original) {
		t.Errorf("comments, key order, or existing values changed: %s", data)
	}
	cfg, err := readConfig(f.configPath)
	if err != nil || cfg.Projects["# next: 世界"] != projectPermalink || cfg.DefaultProject != "work" {
		t.Fatalf("config = %#v, error = %v", cfg, err)
	}
	assertNoConfigTemps(t, target)
}

func TestProjectInitialConfigAndTaskReuse(t *testing.T) {
	for _, initial := range []string{"{}\n", "projects: {} # Empty\n", "projects: # Empty\n", "token_command: [echo, unused]\n"} {
		t.Run(initial, func(t *testing.T) {
			f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/1.0/tasks" {
					_, _ = fmt.Fprintf(w, `{"data":{"permalink_url":%q}}`, testPermalink)
					return
				}
				projectHandler(t, nil)(w, r)
			})
			f.configure(t, initial)
			if code := f.execute("add", "project", "Research", "--alias", "research"); code != 0 {
				t.Fatalf("exit = %d, stderr = %q", code, &f.stderr)
			}
			cfg, err := readConfig(f.configPath)
			if err != nil || cfg.DefaultProject != "" || cfg.Projects["research"] != projectPermalink {
				t.Fatalf("config = %#v, error = %v", cfg, err)
			}
			<-f.requests
			<-f.requests
			f.stdout.Reset()
			r := f.success(t, "add", "task", "Read the proposal", "-p", "research")
			if !strings.Contains(r.body, `"projects":["500"]`) {
				t.Fatalf("body = %s", r.body)
			}
		})
	}
}

func TestProjectConfigOverride(t *testing.T) {
	f := newFixture(t, projectHandler(t, nil))
	f.configure(t, "unknown: key\n")
	defaultPath := f.configPath
	f.configPath = filepath.Join(t.TempDir(), "selected.yaml")
	f.configure(t, "{}\n")
	if code := f.execute("--config", f.configPath, "add", "project", "Research", "--alias", "research"); code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, &f.stderr)
	}
	assertConfigText(t, defaultPath, "unknown: key\n")
	f.stdout.Reset()
	if code := f.execute("--config", f.configPath, "get", "project", "--no-headers"); code != 0 || strings.Join(strings.Fields(f.stdout.String()), " ") != "research 500" {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", code, &f.stdout, &f.stderr)
	}
}

func TestProjectConfigPreparationFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write to read-only directories")
	}
	for _, command := range [][]string{{"add", "project", "Name", "--alias", "new"}, {"get", "project", "--refresh"}} {
		t.Run(strings.Join(command, " "), func(t *testing.T) {
			f := newFixture(t, nil)
			t.Setenv("ASANA_ACCESS_TOKEN", "")
			commandConfig, marker := tokenCommand(t, "token", "", "0")
			f.configure(t, commandConfig+testConfig)
			dir := filepath.Dir(f.configPath)
			if err := os.Chmod(dir, 0o500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
			f.failure(t, f.execute(command...), "prepare config")
			if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) || len(f.requests) != 0 {
				t.Errorf("token marker error = %v, requests = %d", err, len(f.requests))
			}
			assertConfigText(t, f.configPath, commandConfig+testConfig)
		})
	}
}

func TestProjectConfigSaveFailure(t *testing.T) {
	for _, command := range [][]string{{"add", "project", "Name", "--alias", "new"}, {"get", "project", "--refresh"}} {
		t.Run(strings.Join(command, " "), func(t *testing.T) {
			var target string
			f := newFixture(t, projectHandler(t, func(w http.ResponseWriter, r *http.Request) {
				if err := os.Rename(target, target+".original"); err != nil {
					t.Error(err)
				}
				if err := os.Mkdir(target, 0o700); err != nil {
					t.Error(err)
				}
				if command[0] == "add" {
					projectHandler(t, nil)(w, r)
				} else {
					_, _ = fmt.Fprintf(w, `{"data":[{"gid":"500","name":"new","permalink_url":%q,"members":[{"gid":"123"}]}],"next_page":null}`, projectPermalink)
				}
			}))
			target = f.configPath
			code := f.execute(command...)
			if command[0] == "add" {
				f.failure(t, code, "save config", projectPermalink, "new", "manually", "do not repeat")
			} else {
				f.failure(t, code, "save config")
			}
			assertConfigText(t, target+".original", testConfig)
			assertNoConfigTemps(t, target)
		})
	}
}

func TestProjectConfigInterveningEdit(t *testing.T) {
	var target string
	f := newFixture(t, projectHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if err := os.WriteFile(target, []byte(testConfig+"# intervening edit\n"), 0o600); err != nil {
			t.Error(err)
		}
		projectHandler(t, nil)(w, r)
	}))
	target = f.configPath
	if code := f.execute("add", "project", "Research", "--alias", "research"); code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, &f.stderr)
	}
	data, err := os.ReadFile(target)
	if err != nil || strings.Contains(string(data), "intervening edit") {
		t.Fatalf("config = %q, error = %v", data, err)
	}
}

func TestProjectConfigMergedMappings(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprint("explicit=", explicit), func(t *testing.T) {
			f := newFixture(t, projectHandler(t, nil))
			initial := "<<: &settings\n  default_project: work\n  projects: &aliases\n    work: https://app.asana.com/1/100/project/200\n"
			if explicit {
				initial += "projects: *aliases\n"
			}
			f.configure(t, initial)
			if code := f.execute("add", "project", "Research", "--alias", "research"); code != 0 {
				t.Fatalf("exit = %d, stderr = %q", code, &f.stderr)
			}
			cfg, err := readConfig(f.configPath)
			if err != nil || cfg.DefaultProject != "work" || cfg.Projects["work"] != "https://app.asana.com/1/100/project/200" || cfg.Projects["research"] != projectPermalink || len(cfg.Projects) != 2 {
				t.Fatalf("config = %#v, error = %v", cfg, err)
			}
		})
	}
}
