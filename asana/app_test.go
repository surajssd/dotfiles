package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const testConfig = `default_project: work
projects:
  work: https://app.asana.com/1/100/project/200
  personal: https://app.asana.com/0/300/list
`

const testPermalink = "https://app.asana.com/0/200/400"

type recordedRequest struct {
	method, path, query, authorization, contentType, body string
}

type cliFixture struct {
	deps           dependencies
	stdout, stderr bytes.Buffer
	configPath     string
	requests       chan recordedRequest
}

func newFixture(t *testing.T, handler http.HandlerFunc) *cliFixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ASANA_ACCESS_TOKEN", "test-token")
	f := &cliFixture{configPath: filepath.Join(home, ".asana.yaml"), requests: make(chan recordedRequest, 20)}
	f.configure(t, testConfig)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		f.requests <- recordedRequest{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), r.Header.Get("Content-Type"), string(body)}
		if handler != nil {
			handler(w, r)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"data":{"permalink_url":%q}}`, testPermalink)
	}))
	t.Cleanup(server.Close)
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	f.deps = dependencies{
		stdin: strings.NewReader(""), stdout: &f.stdout, stderr: &f.stderr,
		now:        func() time.Time { return time.Date(2026, time.October, 5, 23, 30, 0, 0, location) },
		httpClient: server.Client(), apiBaseURL: server.URL + "/api/1.0",
	}
	f.deps.httpClient.Timeout = 2 * time.Second
	return f
}

func (f *cliFixture) configure(t *testing.T, content string) {
	t.Helper()
	if err := os.WriteFile(f.configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (f *cliFixture) execute(args ...string) int {
	return run(context.Background(), f.deps, args)
}

func (f *cliFixture) success(t *testing.T, args ...string) recordedRequest {
	t.Helper()
	if code := f.execute(args...); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, &f.stderr)
	}
	if f.stdout.String() != testPermalink+"\n" || f.stderr.Len() != 0 {
		t.Fatalf("stdout = %q, stderr = %q", &f.stdout, &f.stderr)
	}
	if len(f.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(f.requests))
	}
	return <-f.requests
}

func (f *cliFixture) failure(t *testing.T, code int, fragments ...string) {
	t.Helper()
	if code != 1 || f.stdout.Len() != 0 || !strings.HasPrefix(f.stderr.String(), "error: ") {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", code, &f.stdout, &f.stderr)
	}
	for _, fragment := range fragments {
		if !strings.Contains(f.stderr.String(), fragment) {
			t.Errorf("stderr = %q, want %q", &f.stderr, fragment)
		}
	}
}

func tokenCommand(t *testing.T, output, stderr, exit string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "token command")
	data, err := os.ReadFile("testdata/token.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, data, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "called")
	t.Setenv("TOKEN_CALL_FILE", marker)
	return fmt.Sprintf("token_command: [%q, %q, %q, %q]\n", script, output, stderr, exit), marker
}

func TestAdd(t *testing.T) {
	cases := []struct {
		name, project, notes, due, stdin string
		args                             []string
	}{
		{name: "default", project: "200"},
		{name: "alias", project: "300", args: []string{"-p", "personal"}},
		{name: "modern URL", project: "500", args: []string{"--project", "https://app.asana.com/1/100/project/500/board?view=1"}},
		{name: "legacy URL", project: "600", args: []string{"-p", "https://app.asana.com/0/600/list"}},
		{name: "description", project: "200", notes: "First line.\nSecond line: 世界", args: []string{"-d", "First line.\nSecond line: 世界"}},
		{name: "stdin", project: "200", notes: "From stdin.\n", stdin: "From stdin.\n", args: []string{"--description", "-"}},
		{name: "date", project: "200", due: "2028-02-29", args: []string{"--due", "2028-02-29"}},
		{name: "today", project: "200", due: "2026-10-05", args: []string{"--due", "today"}},
		{name: "tomorrow", project: "200", due: "2026-10-06", args: []string{"--due", "tomorrow"}},
		{name: "all flags", project: "300", notes: "Details", due: "2026-10-06", args: []string{"-p", "personal", "-d", "Details", "--due", "tomorrow"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, nil)
			f.deps.stdin = strings.NewReader(tc.stdin)
			r := f.success(t, append([]string{"add", "Review the proposal"}, tc.args...)...)
			if r.method != "POST" || r.path != "/api/1.0/tasks" || r.query != "opt_fields=permalink_url" {
				t.Errorf("request = %s %s?%s", r.method, r.path, r.query)
			}
			if r.authorization != "Bearer test-token" || r.contentType != "application/json" {
				t.Errorf("headers = %q, %q", r.authorization, r.contentType)
			}
			data := map[string]any{"name": "Review the proposal", "projects": []any{tc.project}, "assignee": "me", "due_on": "2026-10-05"}
			if tc.notes != "" {
				data["notes"] = tc.notes
			}
			if tc.due != "" {
				data["due_on"] = tc.due
			}
			want := map[string]any{"data": data}
			var got map[string]any
			if err := json.Unmarshal([]byte(r.body), &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("body = %s, want %#v", r.body, want)
			}
		})
	}
}

func TestProjectAliasPrecedesURL(t *testing.T) {
	f := newFixture(t, nil)
	alias := "https://app.asana.com/0/900/list"
	f.configure(t, testConfig+fmt.Sprintf("  %q: https://app.asana.com/0/800/list\n", alias))
	r := f.success(t, "add", "Title", "-p", alias)
	if !strings.Contains(r.body, `"projects":["800"]`) {
		t.Errorf("body = %s", r.body)
	}
}

func TestConfigOverride(t *testing.T) {
	f := newFixture(t, nil)
	f.configure(t, "unknown: key\n")
	f.configPath = filepath.Join(filepath.Dir(f.configPath), "override.yaml")
	f.configure(t, strings.Replace(testConfig, "default_project: work", "default_project: personal", 1))
	r := f.success(t, "--config", f.configPath, "add", "Title")
	if !strings.Contains(r.body, `"projects":["300"]`) {
		t.Errorf("body = %s", r.body)
	}
}

func TestValidationBeforeCredentials(t *testing.T) {
	cases := []struct {
		name, config, want string
		args               []string
	}{
		{name: "missing title", args: []string{"add"}, want: "accepts 1 arg(s)"},
		{name: "extra title", args: []string{"add", "one", "two"}, want: "accepts 1 arg(s)"},
		{name: "blank title", args: []string{"add", " \n\t"}, want: "title must not be blank"},
		{name: "empty title", args: []string{"add", ""}, want: "title must not be blank"},
		{name: "unknown alias", args: []string{"add", "Title", "-p", "unknown"}, want: "configured aliases: personal, work"},
		{name: "bare GID", args: []string{"add", "Title", "-p", "200"}, want: "unknown project"},
		{name: "bad URL", args: []string{"add", "Title", "-p", "https://example.com/0/200/list"}, want: "unknown project"},
		{name: "empty project", args: []string{"add", "Title", "-p", ""}, want: "no project selected"},
		{name: "invalid date", args: []string{"add", "Title", "--due", "2026-02-29"}, want: "invalid due date"},
		{name: "unknown date", args: []string{"add", "Title", "--due", "next week"}, want: "invalid due date"},
		{name: "empty date", args: []string{"add", "Title", "--due", ""}, want: "invalid due date"},
		{name: "unknown key", config: testConfig + "unknown: value\n", want: "field unknown not found"},
		{name: "empty projects", config: "projects: {}\n", want: "projects must not be empty"},
		{name: "missing projects", config: "default_project: work\n", want: "projects must not be empty"},
		{name: "invalid project", config: "projects:\n  work: 200\n", want: "project \"work\""},
		{name: "invalid unused project", config: testConfig + "  broken: https://example.com\n", want: "project \"broken\""},
		{name: "blank alias", config: "projects:\n  ' ': https://app.asana.com/0/200/list\n", want: "aliases must not be blank"},
		{name: "invalid default", config: strings.Replace(testConfig, "default_project: work", "default_project: missing", 1), want: "default_project \"missing\""},
		{name: "unset default", config: strings.Replace(testConfig, "default_project: work\n", "", 1), want: "no project selected"},
		{name: "duplicate key", config: testConfig + "default_project: personal\n", want: "already defined"},
		{name: "extra document", config: testConfig + "---\nprojects: {}\n", want: "one YAML document"},
		{name: "malformed YAML", config: "projects: [\n", want: "parse config"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, nil)
			t.Setenv("ASANA_ACCESS_TOKEN", "")
			command, marker := tokenCommand(t, "token", "", "0")
			cfg := tc.config
			if cfg == "" {
				cfg = testConfig
			}
			f.configure(t, command+cfg)
			args := tc.args
			if args == nil {
				args = []string{"add", "Title"}
			}
			f.failure(t, f.execute(args...), tc.want)
			if len(f.requests) != 0 {
				t.Errorf("requests = %d, want 0", len(f.requests))
			}
			if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("token command ran: %v", err)
			}
		})
	}
}

func TestMissingConfig(t *testing.T) {
	f := newFixture(t, nil)
	t.Setenv("ASANA_ACCESS_TOKEN", "")
	if err := os.Remove(f.configPath); err != nil {
		t.Fatal(err)
	}
	f.failure(t, f.execute("add", "Title"), "read config", ".asana.yaml")
	if len(f.requests) != 0 {
		t.Fatal("request sent with missing config")
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("input failed") }

func TestDescriptionReadFailure(t *testing.T) {
	f := newFixture(t, nil)
	t.Setenv("ASANA_ACCESS_TOKEN", "")
	command, marker := tokenCommand(t, "token", "", "0")
	f.configure(t, command+testConfig)
	f.deps.stdin = brokenReader{}
	f.failure(t, f.execute("add", "Title", "-d", "-"), "read description: input failed")
	if len(f.requests) != 0 {
		t.Fatal("request sent after input failure")
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("token command ran: %v", err)
	}
}

type startedReader struct {
	io.Reader
	started chan struct{}
}

func (r startedReader) Read(p []byte) (int, error) {
	select {
	case r.started <- struct{}{}:
	default:
	}
	return r.Reader.Read(p)
}

func TestDescriptionCancellation(t *testing.T) {
	f := newFixture(t, nil)
	t.Setenv("ASANA_ACCESS_TOKEN", "")
	command, marker := tokenCommand(t, "token", "", "0")
	f.configure(t, command+testConfig)
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close(); _ = writer.Close() }()
	started := make(chan struct{}, 1)
	f.deps.stdin = startedReader{Reader: reader, started: started}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	go func() { done <- run(ctx, f.deps, []string{"add", "Title", "-d", "-"}) }()
	select {
	case <-started:
	case code := <-done:
		t.Fatalf("exited before reading stdin: %d, %s", code, &f.stderr)
	case <-time.After(time.Second):
		_ = writer.Close()
		<-done
		t.Fatal("command did not read stdin")
	}
	cancel()
	select {
	case code := <-done:
		f.failure(t, code, "read description: context canceled")
	case <-time.After(time.Second):
		_ = writer.Close()
		<-done
		t.Fatal("cancellation did not interrupt stdin")
	}
	if len(f.requests) != 0 {
		t.Fatal("request sent after cancellation")
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("token command ran: %v", err)
	}
}

func TestHelpWithoutConfigOrToken(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"help"}, {"add", "--help"}, {"help", "add"}, {"--config", "/missing", "add", "--help"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			f := newFixture(t, nil)
			t.Setenv("ASANA_ACCESS_TOKEN", "")
			if err := os.Remove(f.configPath); err != nil {
				t.Fatal(err)
			}
			if code := f.execute(args...); code != 0 || !strings.Contains(f.stdout.String(), "Usage:") || f.stderr.Len() != 0 {
				t.Fatalf("exit = %d, stdout = %q, stderr = %q", code, &f.stdout, &f.stderr)
			}
			if len(f.requests) != 0 {
				t.Fatal("help sent a request")
			}
		})
	}
}

func TestTokenSources(t *testing.T) {
	cases := []struct {
		name, env, output, stderr, exit, want, failure string
		called                                         bool
	}{
		{name: "environment wins", env: "environment-token", output: "ignored", stderr: "must not run", exit: "7", want: "environment-token"},
		{name: "command", output: "command-token \n\t", exit: "0", want: "command-token", called: true},
		{name: "literal arguments", output: "literal-$HOME-$(false)", exit: "0", want: "literal-$HOME-$(false)", called: true},
		{name: "exit failure", output: "must-not-leak", stderr: "keychain unavailable", exit: "7", failure: "exit status 7; stderr: keychain unavailable", called: true},
		{name: "empty output", output: " \n\t", stderr: "no key found", exit: "0", failure: "empty token; stderr: no key found", called: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, nil)
			t.Setenv("ASANA_ACCESS_TOKEN", tc.env)
			command, marker := tokenCommand(t, tc.output, tc.stderr, tc.exit)
			f.configure(t, command+testConfig)
			if tc.failure == "" {
				r := f.success(t, "add", "Title")
				if r.authorization != "Bearer "+tc.want {
					t.Errorf("authorization = %q", r.authorization)
				}
			} else {
				f.failure(t, f.execute("add", "Title"), tc.failure)
				if len(f.requests) != 0 || strings.Contains(f.stderr.String(), "must-not-leak") {
					t.Fatalf("requests = %d, stderr = %q", len(f.requests), &f.stderr)
				}
			}
			_, err := os.Stat(marker)
			if tc.called && err != nil || !tc.called && !errors.Is(err, os.ErrNotExist) {
				t.Errorf("called = %t, marker error = %v", tc.called, err)
			}
		})
	}
}

func TestUnavailableToken(t *testing.T) {
	for _, tc := range []struct{ config, want string }{
		{"", "set ASANA_ACCESS_TOKEN or configure token_command"},
		{"token_command: [/missing/token-command]\n", "token_command failed"},
		{"token_command: [\"\"]\n", "token_command failed"},
		{"token_command: not-a-list\n", "parse config"},
	} {
		t.Run(tc.config, func(t *testing.T) {
			f := newFixture(t, nil)
			t.Setenv("ASANA_ACCESS_TOKEN", "")
			f.configure(t, tc.config+testConfig)
			f.failure(t, f.execute("add", "Title"), tc.want)
			if len(f.requests) != 0 {
				t.Fatal("request sent without credentials")
			}
		})
	}
}

func TestHTTPFailures(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   []string
	}{
		{401, `{"errors":[{"message":"Not Authorized"}]}`, []string{"authentication failed", "Not Authorized"}},
		{403, `{}`, []string{"permission denied"}},
		{404, `{}`, []string{"project is missing or inaccessible"}},
		{429, `{}`, []string{"rate limit exceeded", "Retry-After: 45"}},
		{500, `{"errors":[{"message":"Server error","phrase":"support phrase"},{"message":"Details"}]}`, []string{"Server error", "support phrase", "Details"}},
		{400, `{"errors":[{"message":"Invalid name"}]}`, []string{"Invalid name"}},
		{502, `<html>Bad gateway</html>`, []string{"Bad Gateway"}},
		{307, `{}`, []string{"HTTP 307"}},
		{201, `{"data":`, []string{"read task response", "task may exist", "before retrying"}},
		{201, `{"data":{}}`, []string{"no permalink_url", "task may exist"}},
	} {
		t.Run(fmt.Sprint(tc.status, tc.body), func(t *testing.T) {
			f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "45")
				w.Header().Set("Location", "/redirected")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			f.failure(t, f.execute("add", "Title"), tc.want...)
			if len(f.requests) != 1 {
				t.Errorf("requests = %d, want 1", len(f.requests))
			}
		})
	}
}

func TestTransportFailure(t *testing.T) {
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close()
	})
	f.failure(t, f.execute("add", "Title"), "task may exist", "check Asana before retrying")
	if len(f.requests) != 1 {
		t.Errorf("requests = %d, want 1", len(f.requests))
	}
}

func TestRequestCancellationAndTimeout(t *testing.T) {
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
			f.failure(t, run(ctx, f.deps, []string{"add", "Title"}), want, "task may exist")
			if len(f.requests) != 1 {
				t.Errorf("requests = %d, want 1", len(f.requests))
			}
		})
	}
}

func TestTokenCommandCancellation(t *testing.T) {
	f := newFixture(t, nil)
	t.Setenv("ASANA_ACCESS_TOKEN", "")
	command, _ := tokenCommand(t, "token", "", "0")
	f.configure(t, command+testConfig)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f.failure(t, run(ctx, f.deps, []string{"add", "Title"}), "token_command failed", "context canceled")
	if len(f.requests) != 0 {
		t.Fatal("request sent after cancellation")
	}
}
