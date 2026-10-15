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
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

const firstVideo = "AAAAAAAAAA1"
const secondVideo = "BBBBBBBBBB2"
const testPlaylistID = "PL_test_playlist"
const fakeBundle = `{"client_id":"fake-client","client_secret":"fake-secret","refresh_token":"fake-refresh"}`

type youtubeRequest struct {
	method, path, query, authorization, contentType, body string
}

type youtubeFixture struct {
	deps           dependencies
	stdout, stderr bytes.Buffer
	requests       chan youtubeRequest
}

func newYouTubeFixture(t *testing.T, override func(http.ResponseWriter, *http.Request) bool) *youtubeFixture {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USER", "playlist-test")
	bin := t.TempDir()
	if err := os.Symlink("/bin/bash", filepath.Join(bin, "bash")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("BASH_ENV", "")
	t.Setenv("ENV", "")
	t.Setenv(credentialService, fakeBundle)
	f := &youtubeFixture{requests: make(chan youtubeRequest, 300)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		f.requests <- youtubeRequest{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), r.Header.Get("Content-Type"), string(body)}
		r.Body = io.NopCloser(bytes.NewReader(body))
		w.Header().Set("Content-Type", "application/json")
		if override != nil && override(w, r) {
			return
		}
		switch r.URL.Path {
		case "/token":
			_, _ = fmt.Fprint(w, `{"access_token":"fake-access","token_type":"Bearer","expires_in":3600}`)
		case "/videos":
			ids := strings.Split(r.URL.Query().Get("id"), ",")
			items := make([]map[string]any, 0, len(ids))
			for index := len(ids) - 1; index >= 0; index-- {
				id := ids[index]
				items = append(items, map[string]any{"id": id, "snippet": map[string]any{"title": "Title " + id, "channelTitle": "Channel " + id}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
		case "/playlists":
			_, _ = fmt.Fprintf(w, `{"id":%q,"status":{"privacyStatus":"private"}}`, testPlaylistID)
		case "/playlistItems":
			var item playlistItem
			if err := json.Unmarshal(body, &item); err != nil {
				t.Errorf("decode insertion: %v", err)
			}
			item.ID = "opaque+item/id="
			_ = json.NewEncoder(w).Encode(item)
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	f.deps = realDependencies()
	f.deps.stdin, f.deps.stdout, f.deps.stderr = strings.NewReader(""), &f.stdout, &f.stderr
	f.deps.httpClient, f.deps.apiBaseURL = server.Client(), server.URL
	f.deps.httpClient.Timeout = 2 * time.Second
	f.deps.oauthEndpoint = oauth2.Endpoint{AuthURL: server.URL + "/authorize", TokenURL: server.URL + "/token", AuthStyle: oauth2.AuthStyleInParams}
	return f
}

func (f *youtubeFixture) execute(args ...string) int {
	return run(context.Background(), f.deps, args)
}

func (f *youtubeFixture) result(t *testing.T, code int, status string) map[string]json.RawMessage {
	t.Helper()
	wantCode := 1
	if status == "created" || status == "validated" {
		wantCode = 0
	}
	if code != wantCode {
		t.Fatalf("exit = %d, want %d; stdout = %s; stderr = %s", code, wantCode, &f.stdout, &f.stderr)
	}
	var result map[string]json.RawMessage
	decoder := json.NewDecoder(strings.NewReader(f.stdout.String()))
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("JSON result: %v; stdout = %q; stderr = %q", err, &f.stdout, &f.stderr)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("extra stdout: %q", &f.stdout)
	}
	if string(result["status"]) != fmt.Sprintf("%q", status) {
		t.Fatalf("status = %s, want %q; stdout = %s; stderr = %s", result["status"], status, &f.stdout, &f.stderr)
	}
	if code == 0 && f.stderr.Len() != 0 || code != 0 && !strings.HasPrefix(f.stderr.String(), "error: ") {
		t.Errorf("unexpected stderr: %q", &f.stderr)
	}
	return result
}

func (f *youtubeFixture) recorded() []youtubeRequest {
	var requests []youtubeRequest
	for len(f.requests) != 0 {
		requests = append(requests, <-f.requests)
	}
	return requests
}

func assertJSONKeys(t *testing.T, value map[string]json.RawMessage, keys ...string) {
	t.Helper()
	if len(value) != len(keys) {
		t.Fatalf("keys = %v, want %v", value, keys)
	}
	for _, key := range keys {
		if _, ok := value[key]; !ok {
			t.Errorf("missing key %s", key)
		}
	}
}

func decodedTracks(t *testing.T, result map[string]json.RawMessage) []map[string]json.RawMessage {
	t.Helper()
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(result["tracks"], &entries); err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestCreateInputModes(t *testing.T) {
	for _, mode := range []string{"flags", "file", "stdin"} {
		t.Run(mode, func(t *testing.T) {
			f := newYouTubeFixture(t, nil)
			input := playlistInput{Title: "  Evening 世界  ", Description: "First line\nSecond line", Tracks: []string{firstVideo, "https://music.youtube.com/watch?v=" + secondVideo + "&list=ignored"}}
			args := []string{"create", "--output", "json"}
			if mode == "flags" {
				args = append(args, "--title", input.Title, "--description", input.Description, "--track", input.Tracks[0], "--track", input.Tracks[1])
			} else {
				data, _ := json.Marshal(input)
				file := "-"
				if mode == "file" {
					file = filepath.Join(t.TempDir(), "playlist.json")
					if err := os.WriteFile(file, data, 0o600); err != nil {
						t.Fatal(err)
					}
				} else {
					f.deps.stdin = bytes.NewReader(data)
				}
				args = append(args, "--file", file)
			}
			result := f.result(t, f.execute(args...), "created")
			assertJSONKeys(t, result, "status", "playlist_id", "url", "privacy", "added", "tracks")
			if string(result["added"]) != "2" || string(result["privacy"]) != `"private"` || string(result["url"]) != fmt.Sprintf("%q", playlistURL(testPlaylistID)) {
				t.Errorf("result = %v", result)
			}
			entries := decodedTracks(t, result)
			if len(entries) != 2 {
				t.Fatalf("entries = %v", entries)
			}
			for index, id := range []string{firstVideo, secondVideo} {
				assertJSONKeys(t, entries[index], "input", "video_id", "title", "channel", "source")
				for key, expected := range map[string]string{"input": input.Tracks[index], "video_id": id, "title": "Title " + id, "channel": "Channel " + id, "source": "exact"} {
					var got string
					if err := json.Unmarshal(entries[index][key], &got); err != nil || got != expected {
						t.Errorf("track %d %s = %s, want %q", index, key, entries[index][key], expected)
					}
				}
			}
			requests := f.recorded()
			var paths []string
			for _, request := range requests {
				paths = append(paths, request.path)
				if request.path != "/token" && request.authorization != "Bearer fake-access" {
					t.Errorf("authorization = %q", request.authorization)
				}
			}
			if !reflect.DeepEqual(paths, []string{"/token", "/videos", "/playlists", "/playlistItems", "/playlistItems"}) {
				t.Fatalf("paths = %v", paths)
			}
			var body map[string]any
			if err := json.Unmarshal([]byte(requests[2].body), &body); err != nil {
				t.Fatal(err)
			}
			want := map[string]any{"snippet": map[string]any{"title": input.Title, "description": input.Description}, "status": map[string]any{"privacyStatus": "private"}}
			if !reflect.DeepEqual(body, want) || requests[2].query != "part=snippet,status" {
				t.Errorf("creation request = %#v, want %#v", requests[2], want)
			}
			for index, id := range []string{firstVideo, secondVideo} {
				request := requests[index+3]
				var body map[string]any
				_ = json.Unmarshal([]byte(request.body), &body)
				want := map[string]any{"snippet": map[string]any{"playlistId": testPlaylistID, "resourceId": map[string]any{"kind": "youtube#video", "videoId": id}}}
				if !reflect.DeepEqual(body, want) || request.query != "part=snippet" || request.method != "POST" || request.contentType != "application/json" {
					t.Errorf("insertion = %#v, want %#v", request, want)
				}
			}
		})
	}
}

func TestURLAndDryRunOutput(t *testing.T) {
	for _, mode := range []string{"url", "preview", "json"} {
		t.Run(mode, func(t *testing.T) {
			f := newYouTubeFixture(t, nil)
			args := []string{"create", "--title", "Evening", "--track", firstVideo}
			if mode != "url" {
				args = append(args, "--dry-run")
			}
			if mode == "json" {
				args = append(args, "--output", "json")
			}
			code := f.execute(args...)
			if mode == "json" {
				result := f.result(t, code, "validated")
				assertJSONKeys(t, result, "status", "title", "description", "privacy", "tracks")
				if string(result["description"]) != `""` {
					t.Errorf("description = %s", result["description"])
				}
			} else if code != 0 || f.stderr.Len() != 0 {
				t.Fatalf("exit = %d, stderr = %s", code, &f.stderr)
			} else if mode == "url" && f.stdout.String() != playlistURL(testPlaylistID)+"\n" {
				t.Errorf("stdout = %q", &f.stdout)
			} else if mode == "preview" && (!strings.Contains(f.stdout.String(), "No playlist was created") || strings.Contains(f.stdout.String(), "playlist?list=")) {
				t.Errorf("preview = %s", &f.stdout)
			}
			for _, request := range f.recorded() {
				if mode != "url" && (request.path == "/playlists" || request.path == "/playlistItems") {
					t.Errorf("dry run mutated: %s", request.path)
				}
				if request.path == "/playlists" && strings.Contains(request.body, "description") {
					t.Errorf("empty description was sent: %s", request.body)
				}
			}
		})
	}
}

func TestJSONValidationFailures(t *testing.T) {
	cases := []struct {
		name, stdin, status, want string
		args                      []string
	}{
		{name: "missing input", status: "error", want: "title"},
		{name: "blank title", status: "error", args: []string{"--title", " ", "--track", firstVideo}, want: "title"},
		{name: "missing tracks", status: "error", args: []string{"--title", "Title"}, want: "track"},
		{name: "conflicting empty title", status: "error", args: []string{"--file", "-", "--title", ""}, want: "combined"},
		{name: "conflicting empty track", status: "error", args: []string{"--file", "-", "--track", ""}, want: "combined"},
		{name: "conflicting empty description", status: "error", args: []string{"--file", "-", "--description", ""}, want: "combined"},
		{name: "empty file flag", status: "error", args: []string{"--file", ""}, want: "file"},
		{name: "missing file", status: "error", args: []string{"--file", "/path/that/does/not/exist"}, want: "input"},
		{name: "unknown JSON key", status: "error", args: []string{"--file", "-"}, stdin: `{"title":"Title","tracks":["AAAAAAAAAA1"],"extra":true}`, want: "input fields"},
		{name: "null description", status: "error", args: []string{"--file", "-"}, stdin: `{"title":"Title","tracks":["AAAAAAAAAA1"],"description":null}`, want: "description"},
		{name: "trailing JSON", status: "error", args: []string{"--file", "-"}, stdin: `{"title":"Title","tracks":["AAAAAAAAAA1"]}{}`, want: "JSON"},
		{name: "invalid URLs and blank", status: "rejected", args: []string{"--title", "Title", "--track", firstVideo, "--track", "https://example.com/watch?v=" + secondVideo, "--track", " "}, want: "rejected"},
		{name: "duplicates", status: "rejected", args: []string{"--title", "Title", "--track", firstVideo, "--track", "https://youtu.be/" + firstVideo}, want: "rejected"},
		{name: "missing search binary", status: "error", args: []string{"--title", "Title", "--track", "Some song and artist"}, want: "brew install yt-dlp"},
		{name: "positional argument", status: "error", args: []string{"extra"}, want: "positional"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newYouTubeFixture(t, nil)
			t.Setenv(credentialService, "")
			f.deps.stdin = strings.NewReader(tc.stdin)
			args := append([]string{"create", "--output", "json"}, tc.args...)
			result := f.result(t, f.execute(args...), tc.status)
			if !strings.Contains(string(result["error"]), tc.want) {
				t.Errorf("error = %s, want %q", result["error"], tc.want)
			}
			if len(f.requests) != 0 {
				t.Errorf("validation made %d requests", len(f.requests))
			}
			if tc.status == "rejected" {
				assertJSONKeys(t, result, "status", "error", "tracks")
				entries := decodedTracks(t, result)
				for index, entry := range entries {
					if string(entry["title"]) != "null" || string(entry["channel"]) != "null" {
						t.Errorf("metadata before preflight = %v", entry)
					}
					if tc.name == "invalid URLs and blank" && string(entry["video_id"]) != "null" {
						t.Errorf("local rejection ID = %s", entry["video_id"])
					}
					_, failed := entry["error"]
					if failed != (tc.name == "duplicates" || index > 0) {
						t.Errorf("error field for track %d = %v", index, failed)
					}
				}
			} else {
				assertJSONKeys(t, result, "status", "error")
			}
		})
	}
}

func TestFlagParsingAndHelp(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"create", "--help"}, {"auth", "--help"}, {"auth", "login", "--help"}, {"auth"}} {
		f := newYouTubeFixture(t, nil)
		t.Setenv(credentialService, "")
		if code := f.execute(args...); code != 0 || f.stdout.Len() == 0 || f.stderr.Len() != 0 || len(f.requests) != 0 {
			t.Fatalf("help %v: exit = %d, stdout = %s, stderr = %s", args, code, &f.stdout, &f.stderr)
		}
	}
	for _, args := range [][]string{{"create", "--output", "json", "--unknown"}, {"create", "--output", "json", "--track"}, {"create", "--output", "json", "--dry-run=invalid"}} {
		f := newYouTubeFixture(t, nil)
		if code := f.execute(args...); code != 1 || f.stdout.Len() != 0 || !strings.Contains(f.stderr.String(), "Usage:") || len(f.requests) != 0 {
			t.Fatalf("parse %v: exit = %d, stdout = %s, stderr = %s", args, code, &f.stdout, &f.stderr)
		}
	}
}

func TestPreflightBatchesAndMissingIDs(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprintf("missing=%t", missing), func(t *testing.T) {
			f := newYouTubeFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
				if missing && r.URL.Path == "/videos" {
					_, _ = fmt.Fprint(w, `{"items":[]}`)
					return true
				}
				return false
			})
			args := []string{"create", "--title", "Many tracks", "--output", "json", "--dry-run"}
			for index := range 51 {
				args = append(args, "--track", fmt.Sprintf("%011d", index))
			}
			status := "validated"
			if missing {
				status = "rejected"
			}
			result := f.result(t, f.execute(args...), status)
			entries := decodedTracks(t, result)
			if len(entries) != 51 {
				t.Fatalf("tracks = %d", len(entries))
			}
			if missing {
				for index, entry := range entries {
					if entry["error"] == nil || string(entry["title"]) != "null" || string(entry["channel"]) != "null" || string(entry["video_id"]) == "null" {
						t.Errorf("missing track %d: %v", index, entry)
					}
				}
			}
			var counts []int
			for _, request := range f.recorded() {
				if request.path == "/videos" {
					query, _ := url.ParseQuery(request.query)
					counts = append(counts, len(strings.Split(query.Get("id"), ",")))
					if query.Get("part") != "snippet" {
						t.Errorf("part = %q", query.Get("part"))
					}
				} else if request.path != "/token" {
					t.Errorf("unexpected request = %s", request.path)
				}
			}
			if !reflect.DeepEqual(counts, []int{50, 1}) {
				t.Errorf("batches = %v", counts)
			}
		})
	}
}

func TestRejectedMetadataPreservesOrder(t *testing.T) {
	f := newYouTubeFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/videos" {
			return false
		}
		_, _ = fmt.Fprintf(w, `{"items":[{"id":%q,"snippet":{"title":"Found title","channelTitle":"Found channel"}}]}`, secondVideo)
		return true
	})
	result := f.result(t, f.execute("create", "--title", "Title", "--track", firstVideo, "--track", secondVideo, "--output", "json"), "rejected")
	entries := decodedTracks(t, result)
	if entries[0]["error"] == nil || string(entries[0]["title"]) != "null" || entries[1]["error"] != nil || string(entries[1]["title"]) != `"Found title"` || string(entries[1]["channel"]) != `"Found channel"` {
		t.Errorf("tracks = %v", entries)
	}
	if len(f.requests) != 2 {
		t.Errorf("requests = %d, want token and preflight only", len(f.requests))
	}
}

func TestCreateFailureOutcomes(t *testing.T) {
	cases := []struct {
		name, path, body, status, reason string
		httpStatus, failItem, added      int
		unknown                          bool
	}{
		{name: "expired auth", path: "/videos", httpStatus: 401, status: "error", reason: "authorization"},
		{name: "preflight quota", path: "/videos", httpStatus: 403, body: `{"error":{"errors":[{"reason":"quotaExceeded"}],"message":"fake-secret"}}`, status: "error", reason: "quota"},
		{name: "create permission", path: "/playlists", httpStatus: 403, status: "error", reason: "permission"},
		{name: "create playlist limit", path: "/playlists", httpStatus: 400, body: `{"error":{"errors":[{"reason":"maxPlaylistExceeded"}]}}`, status: "error", reason: "playlist creation limit"},
		{name: "create rate limit", path: "/playlists", httpStatus: 429, status: "error", reason: "Retry-After: 15"},
		{name: "create server error", path: "/playlists", httpStatus: 503, status: "unknown", reason: "may already exist", unknown: true},
		{name: "create redirect", path: "/playlists", httpStatus: 302, status: "unknown", reason: "redirect", unknown: true},
		{name: "create malformed", path: "/playlists", httpStatus: 200, body: "{", status: "unknown", reason: "unreadable", unknown: true},
		{name: "create missing ID", path: "/playlists", httpStatus: 200, body: `{"status":{"privacyStatus":"private"}}`, status: "unknown", reason: "playlist ID", unknown: true},
		{name: "create invalid ID", path: "/playlists", httpStatus: 200, body: `{"id":"invalid/url","status":{"privacyStatus":"private"}}`, status: "unknown", reason: "playlist ID", unknown: true},
		{name: "privacy mismatch", path: "/playlists", httpStatus: 200, body: `{"id":"PL_test_playlist","status":{"privacyStatus":"public"}}`, status: "partial", reason: "private visibility", unknown: true},
		{name: "first insert denied", path: "/playlistItems", httpStatus: 403, failItem: 1, status: "partial", reason: "permission"},
		{name: "later insert auth", path: "/playlistItems", httpStatus: 401, failItem: 2, added: 1, status: "partial", reason: "authorization"},
		{name: "insert rate limit", path: "/playlistItems", httpStatus: 429, failItem: 1, status: "partial", reason: "Retry-After: 15"},
		{name: "insert service failure", path: "/playlistItems", httpStatus: 500, failItem: 1, status: "partial", reason: "service failed", unknown: true},
		{name: "insert malformed", path: "/playlistItems", httpStatus: 200, body: "{", failItem: 2, added: 1, status: "partial", reason: "unreadable", unknown: true},
		{name: "insert unconfirmed", path: "/playlistItems", httpStatus: 200, body: `{"id":"item"}`, failItem: 1, status: "partial", reason: "confirm", unknown: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items := 0
			f := newYouTubeFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path == "/playlistItems" {
					items++
				}
				if r.URL.Path != tc.path || tc.path == "/playlistItems" && items != tc.failItem {
					return false
				}
				w.Header().Set("Retry-After", "15")
				w.Header().Set("Location", "/must-not-follow")
				w.WriteHeader(tc.httpStatus)
				_, _ = fmt.Fprint(w, tc.body)
				return true
			})
			result := f.result(t, f.execute("create", "--title", "Title", "--track", firstVideo, "--track", secondVideo, "--output", "json"), tc.status)
			if !strings.Contains(string(result["error"]), tc.reason) || strings.Contains(f.stdout.String()+f.stderr.String(), "fake-secret") {
				t.Errorf("error = %s, stderr = %s", result["error"], &f.stderr)
			}
			switch tc.status {
			case "error":
				assertJSONKeys(t, result, "status", "error")
			case "unknown":
				assertJSONKeys(t, result, "status", "error", "outcome_unknown")
			case "partial":
				assertJSONKeys(t, result, "status", "playlist_id", "url", "added", "next_index", "outcome_unknown", "error", "tracks")
				if string(result["added"]) != fmt.Sprint(tc.added) || string(result["next_index"]) != fmt.Sprint(tc.added) || len(decodedTracks(t, result)) != 2 {
					t.Errorf("partial = %v", result)
				}
			}
			if tc.status == "unknown" || tc.status == "partial" {
				if string(result["outcome_unknown"]) != fmt.Sprint(tc.unknown) {
					t.Errorf("outcome_unknown = %s", result["outcome_unknown"])
				}
			}
			counts := make(map[string]int)
			for _, request := range f.recorded() {
				counts[request.path]++
			}
			wantCreate := 1
			if tc.path == "/videos" {
				wantCreate = 0
			}
			if counts["/token"] != 1 || counts["/videos"] != 1 || counts["/playlists"] != wantCreate || counts["/playlistItems"] != tc.failItem || len(counts) > 4 {
				t.Errorf("request counts = %v", counts)
			}
		})
	}
}

func TestDefaultErrorsLeaveStdoutEmpty(t *testing.T) {
	for _, stage := range []string{"local", "credentials", "preflight", "create", "insert"} {
		t.Run(stage, func(t *testing.T) {
			paths := map[string]string{"preflight": "/videos", "create": "/playlists", "insert": "/playlistItems"}
			f := newYouTubeFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path == paths[stage] {
					w.WriteHeader(500)
					return true
				}
				return false
			})
			if stage == "credentials" {
				t.Setenv(credentialService, "invalid")
			}
			id := firstVideo
			if stage == "local" {
				id = "https://example.com/"
			}
			if code := f.execute("create", "--title", "Title", "--track", id); code != 1 || f.stdout.Len() != 0 || !strings.HasPrefix(f.stderr.String(), "error: ") {
				t.Errorf("exit = %d, stdout = %s, stderr = %s", code, &f.stdout, &f.stderr)
			}
			if stage == "insert" {
				for _, text := range []string{playlistURL(testPlaylistID), "next_index: 0", "outcome_unknown: true", firstVideo, "Do not rerun create"} {
					if !strings.Contains(f.stderr.String(), text) {
						t.Errorf("stderr lacks %q: %s", text, &f.stderr)
					}
				}
			}
		})
	}
}

func TestOutputFailurePreservesMutationOutcome(t *testing.T) {
	cases := []struct {
		status, failPath  string
		httpStatus, items int
		want              []string
	}{
		{status: "created", items: 2, want: []string{playlistURL(testPlaylistID), "Confirmed added: 2", "Do not rerun create"}},
		{status: "partial", failPath: "/playlistItems", httpStatus: 403, items: 1, want: []string{playlistURL(testPlaylistID), "Confirmed added: 0", "next_index: 0", "outcome_unknown: false", firstVideo, secondVideo, "Do not rerun create"}},
		{status: "unknown", failPath: "/playlists", httpStatus: 503, want: []string{"may already exist", "inspect your YouTube account before retrying"}},
	}
	for _, output := range []string{"json", "url"} {
		for _, tc := range cases {
			t.Run(output+"/"+tc.status, func(t *testing.T) {
				f := newYouTubeFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
					if r.URL.Path != tc.failPath {
						return false
					}
					w.WriteHeader(tc.httpStatus)
					return true
				})
				reader, writer := io.Pipe()
				_ = reader.Close()
				t.Cleanup(func() { _ = writer.Close() })
				f.deps.stdout = writer
				if code := f.execute("create", "--title", "Title", "--track", firstVideo, "--track", secondVideo, "--output", output); code != 1 {
					t.Fatalf("exit = %d, stderr = %s", code, &f.stderr)
				}
				message := f.stderr.String()
				if strings.Contains(message, io.ErrClosedPipe.Error()) != (output == "json" || tc.status == "created") {
					t.Errorf("output failure diagnostic = %q", message)
				}
				for _, want := range tc.want {
					if !strings.Contains(message, want) {
						t.Errorf("stderr lacks %q: %s", want, message)
					}
				}
				if tc.status == "unknown" && strings.Contains(message, testPlaylistID) {
					t.Errorf("unknown outcome invented a playlist: %s", message)
				}
				counts := make(map[string]int)
				for _, request := range f.recorded() {
					counts[request.path]++
				}
				if counts["/playlists"] != 1 || counts["/playlistItems"] != tc.items {
					t.Errorf("mutation counts = %v", counts)
				}
			})
		}
	}
}

func TestMainClosedStdout(t *testing.T) {
	const helperEnv = "YT_PLAYLIST_CREATOR_TEST_CLOSED_STDOUT"
	if os.Getenv(helperEnv) == "1" {
		os.Args = []string{"yt-playlist-creator", "create", "--output", "json"}
		main()
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	t.Cleanup(func() { _ = writer.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestMainClosedStdout$")
	command.Env = append(os.Environ(), helperEnv+"=1", credentialService+"=invalid", "HOME="+t.TempDir())
	command.Stdout = writer
	var stderr bytes.Buffer
	command.Stderr = &stderr
	var exitError *exec.ExitError
	if err := command.Run(); !errors.As(err, &exitError) || exitError.ExitCode() != 1 {
		t.Fatalf("closed stdout: %v; stderr = %s", err, &stderr)
	}
	if !strings.Contains(stderr.String(), "write JSON result:") {
		t.Errorf("stderr = %q", &stderr)
	}
}

func TestTransportInterruptionsNeverRetryMutations(t *testing.T) {
	for _, path := range []string{"/videos", "/playlists", "/playlistItems"} {
		t.Run(path, func(t *testing.T) {
			f := newYouTubeFixture(t, func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path != path {
					return false
				}
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Errorf("hijack: %v", err)
					return true
				}
				_ = conn.Close()
				return true
			})
			status := map[string]string{"/videos": "error", "/playlists": "unknown", "/playlistItems": "partial"}[path]
			result := f.result(t, f.execute("create", "--title", "Title", "--track", firstVideo, "--output", "json"), status)
			if status != "error" && string(result["outcome_unknown"]) != "true" {
				t.Errorf("outcome = %v", result)
			}
			count := 0
			for _, request := range f.recorded() {
				if request.path == path {
					count++
				}
			}
			if count != 1 {
				t.Errorf("%s requests = %d", path, count)
			}
		})
	}
}

func TestCancellationOutcomes(t *testing.T) {
	for _, stage := range []string{"stdin", "before", "/videos", "/playlists", "/playlistItems"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := newYouTubeFixture(t, func(_ http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path == stage {
					cancel()
					<-r.Context().Done()
					return true
				}
				return false
			})
			args := []string{"create", "--title", "Title", "--track", firstVideo, "--output", "json"}
			if stage == "before" {
				cancel()
			}
			if stage == "stdin" {
				reader, writer := io.Pipe()
				defer func() { _ = reader.Close(); _ = writer.Close() }()
				f.deps.stdin = reader
				args = []string{"create", "--file", "-", "--output", "json"}
				go func() {
					_, _ = writer.Write([]byte(`{"title":`))
					cancel()
				}()
			}
			status := "error"
			switch stage {
			case "/playlists":
				status = "unknown"
			case "/playlistItems":
				status = "partial"
			}
			done := make(chan int, 1)
			go func() { done <- run(ctx, f.deps, args) }()
			select {
			case code := <-done:
				result := f.result(t, code, status)
				if status != "error" && string(result["outcome_unknown"]) != "true" {
					t.Errorf("result = %v", result)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("cancellation did not finish")
			}
		})
	}
}

func TestSearchJSONStages(t *testing.T) {
	cases := []struct {
		name, status string
		tracks       []string
		dryRun       bool
	}{
		{name: "created", status: "created", tracks: []string{firstVideo, "first song", "second song"}},
		{name: "preview", status: "validated", tracks: []string{"first song", firstVideo}, dryRun: true},
		{name: "all unresolved reported", status: "rejected", tracks: []string{"empty result", "first song", "failed result", "second song"}},
		{name: "duplicate queries", status: "rejected", tracks: []string{"second song", "duplicate song"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newYouTubeFixture(t, nil)
			calls := searchStub(t)
			t.Setenv(credentialService, fakeBundle)
			if tc.status == "rejected" {
				t.Setenv(credentialService, "malformed credential value must not be read")
			}
			args := []string{"create", "--title", "Title", "--output", "json"}
			if tc.dryRun {
				args = append(args, "--dry-run")
			}
			for _, text := range tc.tracks {
				args = append(args, "--track", text)
			}
			result := f.result(t, f.execute(args...), tc.status)
			entries := decodedTracks(t, result)
			for index, entry := range entries {
				var input, source string
				_ = json.Unmarshal(entry["input"], &input)
				_ = json.Unmarshal(entry["source"], &source)
				if input != tc.tracks[index] || source != "search" && input != firstVideo {
					t.Errorf("entry %d = %v", index, entry)
				}
				if tc.status == "rejected" {
					failed := tc.name == "duplicate queries" || index == 0 || index == 2
					if (entry["error"] != nil) != failed || string(entry["title"]) != "null" || string(entry["channel"]) != "null" {
						t.Errorf("rejected entry %d = %v", index, entry)
					}
					unresolved := tc.name == "all unresolved reported" && failed
					if (string(entry["video_id"]) == "null") != unresolved {
						t.Errorf("resolved ID %d = %s", index, entry["video_id"])
					}
				} else {
					var id, title, channel string
					_ = json.Unmarshal(entry["video_id"], &id)
					_ = json.Unmarshal(entry["title"], &title)
					_ = json.Unmarshal(entry["channel"], &channel)
					if title != "Title "+id || channel != "Channel "+id || entry["error"] != nil {
						t.Errorf("metadata did not come from preflight: %v", entry)
					}
				}
			}
			if tc.status == "rejected" && len(f.requests) != 0 {
				t.Errorf("rejected search reached account API")
			}
			data, err := os.ReadFile(calls)
			if err != nil || strings.Contains(string(data), "fake-refresh") || strings.Contains(f.stdout.String(), "Search title") {
				t.Errorf("query diagnostic or metadata leak: %v", err)
			}
		})
	}
}

func TestGoogleErrorReasons(t *testing.T) {
	cases := map[string]string{
		"authError": "expired", "insufficientPermissions": "scope", "youtubeSignupRequired": "YouTube channel",
		"videoNotFound": "video", "forbidden": "permission", "quotaExceeded": "quota", "rateLimitExceeded": "rate limit",
		"maxPlaylistExceeded": "playlist creation limit",
	}
	for reason, want := range cases {
		t.Run(reason, func(t *testing.T) {
			resp := &http.Response{StatusCode: 403, Header: make(http.Header)}
			resp.Header.Set("Retry-After", "malicious-secret\tdata")
			message := apiStatusMessage(resp, fmt.Appendf(nil, `{"error":{"errors":[{"reason":%q}],"message":"raw-secret"}}`, reason))
			if !strings.Contains(message, want) || strings.Contains(message, "secret") {
				t.Errorf("message = %s", message)
			}
		})
	}
}
