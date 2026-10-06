package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
)

const membershipProject = `{"gid":"500","name":"Research: #1","permalink_url":"https://app.asana.com/1/100/project/500","members":[{"gid":"999"},{"gid":"123"}]}`

func TestRefreshProjectMemberships(t *testing.T) {
	for _, format := range []string{"table", "wide", "json"} {
		t.Run(format, func(t *testing.T) {
			f := newFixture(t, projectHandler(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("offset") == "next+page/=" {
					_, _ = fmt.Fprintf(w, `{"data":[%s],"next_page":null}`, membershipProject)
					return
				}
				_, _ = io.WriteString(w, `{"data":[
{"gid":"200","name":"Renamed Work","permalink_url":"https://app.asana.com/0/200/list","members":[{"gid":"123"}]},
{"gid":"400","name":"Not a member","permalink_url":"https://app.asana.com/0/400/list","members":[{"gid":"999"}]},
{"gid":"401","name":"Nobody","members":[]}
],"next_page":{"offset":"next+page/=","uri":"https://example.com/untrusted"}}`)
			}))
			if code := f.execute("get", "project", "--refresh", "-o", format); code != 0 || f.stderr.Len() != 0 {
				t.Fatalf("exit = %d, stderr = %q", code, &f.stderr)
			}
			refreshed := f.stdout.String()
			if len(f.requests) != 3 {
				t.Fatalf("requests = %d, want 3", len(f.requests))
			}
			<-f.requests
			for _, offset := range []string{"", "next+page/="} {
				r := <-f.requests
				query, err := url.ParseQuery(r.query)
				if err != nil {
					t.Fatal(err)
				}
				want := url.Values{"workspace": {"100"}, "archived": {"false"}, "limit": {"100"}, "opt_fields": {"name,permalink_url,members.gid"}}
				if offset != "" {
					want.Set("offset", offset)
				}
				if r.method != "GET" || r.path != "/api/1.0/projects" || !reflect.DeepEqual(query, want) || r.authorization != "Bearer test-token" {
					t.Errorf("request = %#v, want query %s", r, want.Encode())
				}
			}
			cfg, err := readConfig(f.configPath)
			wantProjects := map[string]string{"work": "https://app.asana.com/1/100/project/200", "personal": "https://app.asana.com/0/300/list", "Research: #1": projectPermalink}
			if err != nil || !reflect.DeepEqual(cfg.Projects, wantProjects) || cfg.DefaultProject != "work" {
				t.Fatalf("config = %#v, error = %v", cfg, err)
			}
			f.stdout.Reset()
			t.Setenv("ASANA_ACCESS_TOKEN", "")
			if code := f.execute("get", "project", "-o", format); code != 0 || f.stdout.String() != refreshed || f.stderr.Len() != 0 || len(f.requests) != 0 {
				t.Fatalf("exit = %d, stdout = %q, stderr = %q", code, &f.stdout, &f.stderr)
			}
			assertNoConfigTemps(t, f.configPath)
		})
	}
}

func TestRefreshProjectCollision(t *testing.T) {
	for _, projects := range []string{
		membershipProject + `,{"gid":"600","name":"work","permalink_url":"https://app.asana.com/0/600/list","members":[{"gid":"123"}]}`,
		membershipProject + `,{"gid":"600","name":"Research: #1","permalink_url":"https://app.asana.com/0/600/list","members":[{"gid":"123"}]}`,
	} {
		f := newFixture(t, projectHandler(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprintf(w, `{"data":[%s],"next_page":null}`, projects)
		}))
		f.failure(t, f.execute("get", "project", "--refresh"), "600", "conflicts with alias", "configuration was not changed")
		assertConfigText(t, f.configPath, testConfig)
		assertNoConfigTemps(t, f.configPath)
	}
}

func TestRefreshProjectNoChanges(t *testing.T) {
	for _, projects := range []string{"[]", `[{"gid":"200","name":"Renamed","permalink_url":"https://app.asana.com/0/200/list","members":[{"gid":"123"}]}]`} {
		f := newFixture(t, projectHandler(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprintf(w, `{"data":%s,"next_page":null}`, projects)
		}))
		before, err := os.Stat(f.configPath)
		if err != nil {
			t.Fatal(err)
		}
		if code := f.execute("get", "project", "--refresh"); code != 0 || f.stderr.Len() != 0 {
			t.Fatalf("exit = %d, stderr = %q", code, &f.stderr)
		}
		after, err := os.Stat(f.configPath)
		if err != nil || !os.SameFile(before, after) {
			t.Fatalf("config was replaced with no changes; error = %v", err)
		}
		assertConfigText(t, f.configPath, testConfig)
		assertNoConfigTemps(t, f.configPath)
	}
}

func TestRefreshProjectFailures(t *testing.T) {
	for _, tc := range []struct {
		status     int
		body, want string
	}{
		{401, `{}`, "authentication failed"},
		{403, `{}`, "permission denied"},
		{404, `{}`, "missing or inaccessible"},
		{429, `{}`, "Retry-After: 45"},
		{500, `{"errors":[{"message":"Server error"}]}`, "Server error"},
		{307, `{}`, "HTTP 307"},
		{200, `{"data":`, "read Asana response"},
		{200, `{}`, "no project data"},
		{200, `{"data":[],"next_page":{}}`, "missing pagination offset"},
		{200, `{"data":[],"next_page":{"offset":"next"}}`, "repeated pagination offset"},
		{200, `{"data":[{"gid":"600","name":"Bad URL","permalink_url":"https://example.com","members":[{"gid":"123"}]}]}`, "invalid Asana project URL"},
		{200, `{"data":[{"gid":"600","name":"Bad GID","permalink_url":"https://app.asana.com/0/601/list","members":[{"gid":"123"}]}]}`, "invalid name or GID"},
		{200, `{"data":[{"gid":"600","name":" ","permalink_url":"https://app.asana.com/0/600/list","members":[{"gid":"123"}]}]}`, "invalid name or GID"},
	} {
		t.Run(fmt.Sprint(tc.status, tc.body), func(t *testing.T) {
			f := newFixture(t, projectHandler(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("offset") == "" {
					_, _ = fmt.Fprintf(w, `{"data":[%s],"next_page":{"offset":"next"}}`, membershipProject)
					return
				}
				w.Header().Set("Retry-After", "45")
				w.Header().Set("Location", "/redirected")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			f.failure(t, f.execute("get", "project", "--refresh"), tc.want)
			if len(f.requests) != 3 || strings.Contains(f.stderr.String(), "may exist") {
				t.Fatalf("requests = %d, stderr = %q", len(f.requests), &f.stderr)
			}
			assertConfigText(t, f.configPath, testConfig)
			assertNoConfigTemps(t, f.configPath)
		})
	}
}

func TestRefreshProjectRequiresUserGID(t *testing.T) {
	f := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"workspaces":[{"gid":"100"}]}}`)
	})
	f.failure(t, f.execute("get", "project", "--refresh"), "invalid user GID")
	if len(f.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(f.requests))
	}
	assertConfigText(t, f.configPath, testConfig)
	assertNoConfigTemps(t, f.configPath)
}

func TestProjectAliasWhitespace(t *testing.T) {
	f := newFixture(t, projectHandler(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"gid":"500","name":" Research\n\tNotes ","permalink_url":"https://app.asana.com/0/500/list","members":[{"gid":"123"}]}],"next_page":null}`)
	}))
	f.configure(t, "{}\n")
	if code := f.execute("get", "project", "--refresh", "--no-headers"); code != 0 || f.stderr.Len() != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, &f.stderr)
	}
	if strings.Count(f.stdout.String(), "\n") != 1 || strings.ContainsAny(f.stdout.String(), "\t\r") {
		t.Fatalf("table has multiline cells: %q", &f.stdout)
	}
	f.stdout.Reset()
	if code := f.execute("get", "project", "-o", "json"); code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, &f.stderr)
	}
	var result struct{ Items []struct{ Alias string } }
	if err := json.Unmarshal(f.stdout.Bytes(), &result); err != nil || len(result.Items) != 1 || result.Items[0].Alias != " Research\n\tNotes " {
		t.Fatalf("output = %s, error = %v", &f.stdout, err)
	}
}
