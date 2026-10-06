package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

const projectPermalink = "https://app.asana.com/1/100/project/500"
const projectUser = `{"data":{"gid":"123","workspaces":[{"gid":"100"}]}}`

func projectHandler(t *testing.T, handler http.HandlerFunc) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/1.0/users/me" {
			_, _ = io.WriteString(w, projectUser)
			return
		}
		if r.URL.Path != "/api/1.0/projects" {
			t.Errorf("unexpected request: %s", r.URL)
		}
		if handler != nil {
			handler(w, r)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"data":{"privacy_setting":"private","permalink_url":%q}}`, projectPermalink)
	}
}

func TestCommandParents(t *testing.T) {
	for _, args := range [][]string{{"add"}, {"get"}, {"add", "project", "--help"}, {"get", "project", "--help"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			f := newFixture(t, nil)
			t.Setenv("ASANA_ACCESS_TOKEN", "")
			if err := os.Remove(f.configPath); err != nil {
				t.Fatal(err)
			}
			if code := f.execute(args...); code != 0 || !strings.Contains(f.stdout.String(), "Usage:") || f.stderr.Len() != 0 || len(f.requests) != 0 {
				t.Fatalf("exit = %d, stdout = %q, stderr = %q", code, &f.stdout, &f.stderr)
			}
		})
	}
	for _, args := range [][]string{{"add", "Title"}, {"get", "--due", "any"}} {
		f := newFixture(t, nil)
		f.failure(t, f.execute(args...), "unknown")
		if len(f.requests) != 0 {
			t.Fatal("old syntax sent a request")
		}
	}
}

func TestAddProject(t *testing.T) {
	for _, workspace := range []string{"", "101"} {
		t.Run("workspace="+workspace, func(t *testing.T) {
			f := newFixture(t, projectHandler(t, nil))
			t.Setenv("ASANA_ACCESS_TOKEN", "")
			command, _ := tokenCommand(t, "project-token\n", "", "0")
			f.configure(t, command+testConfig)
			args := []string{"add", "project", " Research: #1 ", "--alias", "release: #1"}
			if workspace != "" {
				args = append(args, "--workspace", workspace)
			}
			if code := f.execute(args...); code != 0 || f.stdout.String() != projectPermalink+"\n" || f.stderr.Len() != 0 {
				t.Fatalf("exit = %d, stdout = %q, stderr = %q", code, &f.stdout, &f.stderr)
			}
			if len(f.requests) != 2 {
				t.Fatalf("requests = %d, want 2", len(f.requests))
			}
			userRequest := <-f.requests
			if userRequest.path != "/api/1.0/users/me" || userRequest.query != "opt_fields=workspaces" {
				t.Errorf("user request = %#v", userRequest)
			}
			r := <-f.requests
			query, err := url.ParseQuery(r.query)
			if err != nil {
				t.Fatal(err)
			}
			if r.method != "POST" || r.path != "/api/1.0/projects" || query.Get("opt_fields") != "permalink_url,privacy_setting" || r.authorization != "Bearer project-token" || r.contentType != "application/json" {
				t.Errorf("request = %#v", r)
			}
			if workspace == "" {
				workspace = "100"
			}
			var body map[string]map[string]string
			if err := json.Unmarshal([]byte(r.body), &body); err != nil {
				t.Fatal(err)
			}
			want := map[string]map[string]string{"data": {"name": " Research: #1 ", "workspace": workspace, "privacy_setting": "private"}}
			if !reflect.DeepEqual(body, want) {
				t.Errorf("body = %#v, want %#v", body, want)
			}
			cfg, err := readConfig(f.configPath)
			if err != nil || cfg.Projects["release: #1"] != projectPermalink || cfg.DefaultProject != "work" || len(cfg.Projects) != 3 {
				t.Fatalf("config = %#v, error = %v", cfg, err)
			}
		})
	}
}

func TestProjectValidationBeforeCredentials(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"add", "project"}, "accepts 1 arg(s)"},
		{[]string{"add", "project", "one", "two", "--alias", "new"}, "accepts 1 arg(s)"},
		{[]string{"add", "project", " \n", "--alias", "new"}, "name must not be blank"},
		{[]string{"add", "project", "Name"}, "alias"},
		{[]string{"add", "project", "Name", "--alias", " \t"}, "alias must not be blank"},
		{[]string{"add", "project", "Name", "--alias", "work"}, "already"},
		{[]string{"add", "project", "Name", "--alias", "new", "--workspace", ""}, "workspace"},
		{[]string{"add", "project", "Name", "--alias", "new", "--workspace", "abc"}, "workspace"},
		{[]string{"add", "project", "Name", "--alias", "new", "-d", "text"}, "unknown shorthand flag"},
		{[]string{"get", "project", "extra"}, "unknown command"},
		{[]string{"get", "project", "--workspace", "100"}, "requires --refresh"},
		{[]string{"get", "project", "--refresh", "--workspace", "100x"}, "workspace"},
		{[]string{"get", "project", "--refresh", "--workspace", ""}, "workspace"},
		{[]string{"get", "project", "--refresh", "-o", "yaml"}, "use table, wide, or json"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			f := newFixture(t, nil)
			t.Setenv("ASANA_ACCESS_TOKEN", "")
			command, marker := tokenCommand(t, "token", "", "0")
			f.configure(t, command+testConfig)
			f.failure(t, f.execute(tc.args...), tc.want)
			if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) || len(f.requests) != 0 {
				t.Errorf("token marker error = %v, requests = %d", err, len(f.requests))
			}
		})
	}
}

func TestGetProjectLocal(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "ALIAS GID personal 300 work 200"},
		{[]string{"-o", "wide"}, "ALIAS GID URL personal 300 https://app.asana.com/0/300/list work 200 https://app.asana.com/1/100/project/200"},
		{[]string{"--no-headers"}, "personal 300 work 200"},
		{[]string{"-o", "wide", "--no-headers"}, "personal 300 https://app.asana.com/0/300/list work 200 https://app.asana.com/1/100/project/200"},
		{[]string{"-o", "json", "--no-headers"}, `{"items":[{"alias":"personal","gid":"300","url":"https://app.asana.com/0/300/list"},{"alias":"work","gid":"200","url":"https://app.asana.com/1/100/project/200"}]}`},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			f := newFixture(t, nil)
			t.Setenv("ASANA_ACCESS_TOKEN", "")
			command, marker := tokenCommand(t, "", "must not run", "7")
			f.configure(t, command+testConfig)
			if code := f.execute(append([]string{"get", "project"}, tc.args...)...); code != 0 || f.stderr.Len() != 0 {
				t.Fatalf("exit = %d, stderr = %q", code, &f.stderr)
			}
			if got := strings.Join(strings.Fields(f.stdout.String()), " "); got != tc.want {
				t.Errorf("stdout = %q, want %q", got, tc.want)
			}
			if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) || len(f.requests) != 0 {
				t.Errorf("token marker error = %v, requests = %d", err, len(f.requests))
			}
		})
	}
}

func TestGetProjectEmpty(t *testing.T) {
	for _, cfg := range []string{"{}\n", "projects: {}\n", "projects:\n"} {
		for _, format := range []string{"table", "wide", "json"} {
			t.Run(cfg+format, func(t *testing.T) {
				f := newFixture(t, nil)
				f.configure(t, cfg)
				if code := f.execute("get", "project", "-o", format); code != 0 {
					t.Fatalf("exit = %d, stderr = %q", code, &f.stderr)
				}
				if format == "json" {
					if f.stdout.String() != "{\"items\":[]}\n" || f.stderr.Len() != 0 {
						t.Fatalf("stdout = %q, stderr = %q", &f.stdout, &f.stderr)
					}
				} else if f.stdout.Len() != 0 || f.stderr.String() != "No projects configured.\n" {
					t.Fatalf("stdout = %q, stderr = %q", &f.stdout, &f.stderr)
				}
			})
		}
	}
}

func TestProjectWorkspaceSelection(t *testing.T) {
	for _, command := range [][]string{{"add", "project", "Name", "--alias", "new"}, {"get", "project", "--refresh"}} {
		for _, tc := range []struct {
			body, want string
			workspace  string
		}{
			{`{"data":{"gid":"123","workspaces":[]}}`, "no accessible workspaces", ""},
			{`{"data":{"gid":"123","workspaces":[{"gid":"100"},{"gid":"101"}]}}`, "100, 101", ""},
			{`{"data":{"gid":"123","workspaces":[{"gid":"100"},{"gid":"101"}]}}`, "", "101"},
			{`{"data":{"gid":"123"}}`, "no workspaces", ""},
			{`{"data":{"gid":"123","workspaces":[{"gid":"bad"}]}}`, "invalid workspace GID", ""},
		} {
			t.Run(strings.Join(command, " ")+tc.body+tc.workspace, func(t *testing.T) {
				f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/1.0/users/me" {
						_, _ = io.WriteString(w, tc.body)
						return
					}
					if command[0] == "get" {
						if r.URL.Query().Get("workspace") != tc.workspace {
							t.Errorf("workspace = %s", r.URL.Query().Get("workspace"))
						}
						_, _ = io.WriteString(w, `{"data":[],"next_page":null}`)
					} else {
						projectHandler(t, nil)(w, r)
					}
				})
				args := append([]string{}, command...)
				if tc.workspace != "" {
					args = append(args, "--workspace", tc.workspace)
				}
				code := f.execute(args...)
				if tc.want != "" {
					f.failure(t, code, tc.want)
					if len(f.requests) != 1 {
						t.Fatalf("requests = %d, want 1", len(f.requests))
					}
				} else if code != 0 || f.stderr.Len() != 0 || len(f.requests) != 2 {
					t.Fatalf("exit = %d, stderr = %q, requests = %d", code, &f.stderr, len(f.requests))
				}
			})
		}
	}
}

func TestAddProjectFailures(t *testing.T) {
	for _, tc := range []struct {
		status     int
		body, want string
	}{
		{401, `{}`, "authentication failed"},
		{403, `{}`, "permission denied"},
		{404, `{}`, "workspace is missing or inaccessible"},
		{429, `{}`, "Retry-After: 45"},
		{400, `{"errors":[{"message":"Invalid privacy"}]}`, "Invalid privacy"},
		{500, `{"errors":[{"phrase":"support phrase"}]}`, "support phrase"},
		{307, `{}`, "HTTP 307"},
		{201, `{"data":`, "project may exist"},
		{201, `{"data":{"privacy_setting":"private"}}`, "project may exist"},
		{201, `{"data":{"privacy_setting":"private","permalink_url":"https://example.com"}}`, "project may exist"},
		{201, `{"data":{"privacy_setting":"public_to_workspace","permalink_url":"https://app.asana.com/1/100/project/500"}}`, "private"},
		{201, `{"data":{"permalink_url":"https://app.asana.com/1/100/project/500"}}`, "private"},
	} {
		t.Run(fmt.Sprint(tc.status, tc.body), func(t *testing.T) {
			f := newFixture(t, projectHandler(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "45")
				w.Header().Set("Location", "/redirected")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			f.failure(t, f.execute("add", "project", "Name", "--alias", "new"), tc.want)
			if len(f.requests) != 2 {
				t.Fatalf("requests = %d, want 2", len(f.requests))
			}
			assertConfigText(t, f.configPath, testConfig)
			assertNoConfigTemps(t, f.configPath)
		})
	}
}

func TestProjectRequestCancellationAndTimeout(t *testing.T) {
	for _, command := range [][]string{{"add", "project", "Name", "--alias", "new"}, {"get", "project", "--refresh"}} {
		for _, timeout := range []bool{false, true} {
			t.Run(fmt.Sprint(command, timeout), func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				f := newFixture(t, projectHandler(t, func(w http.ResponseWriter, r *http.Request) {
					if !timeout {
						cancel()
					}
					<-r.Context().Done()
				}))
				want := "context canceled"
				if timeout {
					f.deps.httpClient.Timeout = 100 * time.Millisecond
					want = "deadline exceeded"
				}
				f.failure(t, run(ctx, f.deps, command), want)
				if (command[0] == "add") != strings.Contains(f.stderr.String(), "project may exist") || len(f.requests) != 2 {
					t.Fatalf("requests = %d, stderr = %q", len(f.requests), &f.stderr)
				}
				assertConfigText(t, f.configPath, testConfig)
				assertNoConfigTemps(t, f.configPath)
			})
		}
	}
}

func TestProjectTransportFailure(t *testing.T) {
	for _, command := range [][]string{{"add", "project", "Name", "--alias", "new"}, {"get", "project", "--refresh"}} {
		t.Run(strings.Join(command, " "), func(t *testing.T) {
			f := newFixture(t, projectHandler(t, func(w http.ResponseWriter, r *http.Request) {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				_ = conn.Close()
			}))
			f.failure(t, f.execute(command...), "EOF")
			if (command[0] == "add") != strings.Contains(f.stderr.String(), "project may exist") || len(f.requests) != 2 {
				t.Fatalf("requests = %d, stderr = %q", len(f.requests), &f.stderr)
			}
			assertConfigText(t, f.configPath, testConfig)
			assertNoConfigTemps(t, f.configPath)
		})
	}
}

func TestProjectTokenFailure(t *testing.T) {
	for _, command := range [][]string{{"add", "project", "Name", "--alias", "new"}, {"get", "project", "--refresh"}} {
		t.Run(strings.Join(command, " "), func(t *testing.T) {
			f := newFixture(t, nil)
			t.Setenv("ASANA_ACCESS_TOKEN", "")
			credentialConfig, _ := tokenCommand(t, "must-not-leak", "keychain unavailable", "7")
			f.configure(t, credentialConfig+testConfig)
			f.failure(t, f.execute(command...), "token_command failed", "keychain unavailable")
			if len(f.requests) != 0 || strings.Contains(f.stderr.String(), "must-not-leak") {
				t.Fatalf("requests = %d, stderr = %q", len(f.requests), &f.stderr)
			}
			assertConfigText(t, f.configPath, credentialConfig+testConfig)
			assertNoConfigTemps(t, f.configPath)
		})
	}
}
