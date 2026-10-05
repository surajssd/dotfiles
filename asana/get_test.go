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

const listingTasks = `[
  {"gid":"6","name":"Undated","due_on":null,"due_at":null,"permalink_url":"url6","projects":[]},
  {"gid":"5","name":"Tomorrow","due_on":"2026-10-06","due_at":null,"permalink_url":"url5","projects":[{"gid":"300","name":"Personal"}]},
  {"gid":"4","name":"Today","due_on":"2026-10-05","due_at":null,"permalink_url":"url4","projects":[{"gid":"200","name":"Work"},{"gid":"300","name":"Personal"}]},
  {"gid":"3","name":"Local\n\tdate","due_on":"2026-10-06","due_at":"2026-10-06T06:30:00Z","permalink_url":"url3","projects":[{"gid":"300","name":"Personal\n\tproject"}]},
  {"gid":"2","name":"Yesterday","due_on":"2026-10-04","due_at":null,"permalink_url":"url2","projects":[{"gid":"200","name":"Work"}]},
  {"gid":"1","name":"Yesterday","due_on":"2026-10-04","due_at":null,"permalink_url":"url1","projects":[]}
]`

func listingHandler(t *testing.T, tasks string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/1.0/users/me" {
			_, _ = io.WriteString(w, `{"data":{"workspaces":[{"gid":"100"}]}}`)
			return
		}
		if r.URL.Path != "/api/1.0/tasks" {
			t.Errorf("unexpected request: %s", r.URL)
		}
		_, _ = fmt.Fprintf(w, `{"data":%s,"next_page":null}`, tasks)
	}
}

func listedItems(t *testing.T, f *cliFixture, args ...string) []map[string]any {
	t.Helper()
	if code := f.execute(append([]string{"get", "-o", "json"}, args...)...); code != 0 || f.stderr.Len() != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, &f.stderr)
	}
	var result struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(f.stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Items == nil {
		t.Fatal("items must be an array")
	}
	return result.Items
}

func TestGetFilters(t *testing.T) {
	for _, tc := range []struct {
		name, gids string
		args       []string
	}{
		{"default", "1 2 3 4", nil},
		{"today", "1 2 3 4", []string{"--due", "today"}},
		{"yesterday", "1 2", []string{"--due", "yesterday"}},
		{"tomorrow", "1 2 3 4 5", []string{"--due", "tomorrow"}},
		{"date", "1 2", []string{"--due", "2026-10-04"}},
		{"any", "1 2 3 4 5 6", []string{"--due", "any"}},
		{"none", "6", []string{"--due", "none"}},
		{"alias", "2 4", []string{"-p", "work"}},
		{"URL", "3 4", []string{"-p", "https://app.asana.com/1/100/project/300/list?view=a,b"}},
		{"repeated projects", "2 3 4", []string{"-p", "work", "-p", "personal", "-p", "work"}},
		{"combined filters", "3 4 5", []string{"--due", "tomorrow", "-p", "personal"}},
		{"no matches", "", []string{"--due", "2026-10-03"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, listingHandler(t, listingTasks))
			var gids []string
			for _, item := range listedItems(t, f, tc.args...) {
				gids = append(gids, item["gid"].(string))
			}
			if got := strings.Join(gids, " "); got != tc.gids {
				t.Errorf("gids = %q, want %q", got, tc.gids)
			}
		})
	}
}

func TestGetOutput(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"default", nil, "GID PROJECT DUE NAME\n1 - 2026-10-04 Yesterday\n2 Work 2026-10-04 Yesterday\n3 Personal project 2026-10-05 Local date\n4 Work,Personal 2026-10-05 Today"},
		{"table", []string{"-o", "table", "--due", "none"}, "GID PROJECT DUE NAME\n6 - - Undated"},
		{"wide", []string{"--output", "wide", "--due", "none"}, "GID PROJECT DUE NAME URL\n6 - - Undated url6"},
		{"no headers", []string{"--no-headers", "--due", "none"}, "6 - - Undated"},
		{"wide no headers", []string{"-o", "wide", "--no-headers", "--due", "none"}, "6 - - Undated url6"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, listingHandler(t, listingTasks))
			if code := f.execute(append([]string{"get"}, tc.args...)...); code != 0 || f.stderr.Len() != 0 {
				t.Fatalf("exit = %d, stderr = %q", code, &f.stderr)
			}
			lines := strings.Split(strings.TrimSuffix(f.stdout.String(), "\n"), "\n")
			for i, line := range lines {
				lines[i] = strings.Join(strings.Fields(line), " ")
			}
			if got := strings.Join(lines, "\n"); got != tc.want || strings.ContainsAny(f.stdout.String(), "\t\r") {
				t.Errorf("stdout = %q, want rows %q", &f.stdout, tc.want)
			}
		})
	}
	f := newFixture(t, listingHandler(t, listingTasks))
	items := listedItems(t, f, "--due", "any", "--no-headers")
	var original []map[string]any
	if err := json.Unmarshal([]byte(listingTasks), &original); err != nil {
		t.Fatal(err)
	}
	for i, item := range items {
		if !reflect.DeepEqual(item, original[len(original)-1-i]) {
			t.Errorf("item = %#v, want %#v", item, original[len(original)-1-i])
		}
	}
}

func TestGetEmpty(t *testing.T) {
	for _, format := range []string{"table", "wide", "json"} {
		t.Run(format, func(t *testing.T) {
			f := newFixture(t, listingHandler(t, `[]`))
			if code := f.execute("get", "-o", format); code != 0 {
				t.Fatalf("exit = %d, stderr = %q", code, &f.stderr)
			}
			if format == "json" {
				if strings.TrimSpace(f.stdout.String()) != `{"items":[]}` || f.stderr.Len() != 0 {
					t.Fatalf("stdout = %q, stderr = %q", &f.stdout, &f.stderr)
				}
			} else if f.stdout.Len() != 0 || f.stderr.String() != "No tasks found.\n" {
				t.Fatalf("stdout = %q, stderr = %q", &f.stdout, &f.stderr)
			}
		})
	}
}

func TestGetPagination(t *testing.T) {
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/1.0/users/me":
			_, _ = io.WriteString(w, `{"data":{"workspaces":[{"gid":"100"},{"gid":"101"}]}}`)
		case r.URL.Query().Get("workspace") == "101":
			_, _ = io.WriteString(w, `{"data":[{"gid":"1","name":"First"}],"next_page":null}`)
		case r.URL.Query().Get("offset") == "next+page/=":
			_, _ = io.WriteString(w, `{"data":[{"gid":"2","name":"Second"}],"next_page":null}`)
		default:
			_, _ = io.WriteString(w, `{"data":[{"gid":"3","name":"Third"}],"next_page":{"offset":"next+page/=","uri":"https://example.com/untrusted"}}`)
		}
	})
	items := listedItems(t, f, "--due", "any")
	if len(items) != 3 || items[0]["gid"] != "1" || items[1]["gid"] != "2" || items[2]["gid"] != "3" {
		t.Fatalf("items = %#v", items)
	}
	if len(f.requests) != 4 {
		t.Fatalf("requests = %d, want 4", len(f.requests))
	}
	for i, workspace := range []string{"", "100", "100", "101"} {
		r := <-f.requests
		query, err := url.ParseQuery(r.query)
		if err != nil {
			t.Fatal(err)
		}
		want := url.Values{"opt_fields": {"workspaces"}}
		path := "/api/1.0/users/me"
		if workspace != "" {
			path = "/api/1.0/tasks"
			want = url.Values{"assignee": {"me"}, "workspace": {workspace}, "completed_since": {"now"}, "limit": {"100"}, "opt_fields": {"name,due_on,due_at,permalink_url,projects.name"}}
		}
		if i == 2 {
			want.Set("offset", "next+page/=")
		}
		if r.method != "GET" || r.path != path || r.authorization != "Bearer test-token" || r.body != "" || !reflect.DeepEqual(query, want) {
			t.Errorf("request = %#v, want %s?%s", r, path, want.Encode())
		}
	}
}

func TestGetFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		status           int
	}{
		{"authentication", `{"errors":[{"message":"Not Authorized"}]}`, "authentication failed: check ASANA_ACCESS_TOKEN or token_command; Not Authorized", 401},
		{"permission", `{}`, "permission denied", 403},
		{"missing", `{}`, "missing or inaccessible", 404},
		{"rate limit", `{}`, "rate limit exceeded; Retry-After: 45", 429},
		{"server", `{"errors":[{"message":"Server error","phrase":"support phrase"}]}`, "Server error; support phrase", 500},
		{"redirect", `{}`, "HTTP 307", 307},
		{"malformed JSON", `{"data":`, "read Asana response", 200},
		{"missing data", `{}`, "no task data", 200},
		{"missing offset", `{"data":[],"next_page":{}}`, "missing pagination offset", 200},
		{"repeated offset", `{"data":[],"next_page":{"offset":"next"}}`, "repeated pagination offset", 200},
		{"invalid timestamp", `{"data":[{"gid":"1","due_at":"invalid"}]}`, "due_at", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/1.0/users/me" {
					_, _ = io.WriteString(w, `{"data":{"workspaces":[{"gid":"100"}]}}`)
					return
				}
				if r.URL.Query().Get("offset") == "" {
					_, _ = io.WriteString(w, `{"data":[{"gid":"1","name":"Already fetched","due_on":"2026-10-05"}],"next_page":{"offset":"next"}}`)
					return
				}
				w.Header().Set("Retry-After", "45")
				w.Header().Set("Location", "/redirected")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			f.failure(t, f.execute("get"), tc.want)
			if len(f.requests) != 3 || strings.Contains(f.stderr.String(), "task may exist") {
				t.Errorf("requests = %d, stderr = %q", len(f.requests), &f.stderr)
			}
		})
	}
}

func TestGetValidationBeforeCredentials(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"title"}, "unknown command"},
		{[]string{"--due", "invalid"}, "use YYYY-MM-DD, yesterday, today, tomorrow, any, or none"},
		{[]string{"--due", "2026-02-29"}, "invalid due date"},
		{[]string{"--due", ""}, "invalid due date"},
		{[]string{"-p", "work", "-p", "unknown"}, "unknown project"},
		{[]string{"-p", ""}, "no project selected"},
		{[]string{"-o", "yaml"}, "use table, wide, or json"},
		{[]string{"--all"}, "unknown flag"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			f := newFixture(t, nil)
			t.Setenv("ASANA_ACCESS_TOKEN", "")
			command, marker := tokenCommand(t, "token", "", "0")
			f.configure(t, command+testConfig)
			f.failure(t, f.execute(append([]string{"get"}, tc.args...)...), tc.want)
			if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) || len(f.requests) != 0 {
				t.Errorf("token marker error = %v, requests = %d", err, len(f.requests))
			}
		})
	}
}

func TestGetCancellationAndTimeout(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(fmt.Sprint("timeout=", timeout), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if !timeout {
					cancel()
				}
				<-r.Context().Done()
			})
			want := "context canceled"
			if timeout {
				f.deps.httpClient.Timeout = 100 * time.Millisecond
				want = "deadline exceeded"
			}
			f.failure(t, run(ctx, f.deps, []string{"get"}), want)
			if len(f.requests) != 1 || strings.Contains(f.stderr.String(), "task may exist") {
				t.Errorf("requests = %d, stderr = %q", len(f.requests), &f.stderr)
			}
		})
	}
}

func TestGetHelpWithoutConfigOrToken(t *testing.T) {
	f := newFixture(t, nil)
	t.Setenv("ASANA_ACCESS_TOKEN", "")
	if err := os.Remove(f.configPath); err != nil {
		t.Fatal(err)
	}
	if code := f.execute("get", "--help"); code != 0 || !strings.Contains(f.stdout.String(), "Usage:") || f.stderr.Len() != 0 || len(f.requests) != 0 {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q, requests = %d", code, &f.stdout, &f.stderr, len(f.requests))
	}
}
