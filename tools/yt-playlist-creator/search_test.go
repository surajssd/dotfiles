package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func searchStub(t *testing.T) string {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(bash, filepath.Join(dir, "bash")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("testdata/search.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "yt-dlp"), data, 0o700); err != nil {
		t.Fatal(err)
	}
	calls := filepath.Join(dir, "calls")
	t.Setenv("PATH", dir)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BASH_ENV", "")
	t.Setenv("ENV", "")
	t.Setenv("SEARCH_CALLS", calls)
	t.Setenv("YT_PLAYLIST_CREATOR_OAUTH", "SECRET")
	return calls
}

func searchInputs(t *testing.T, inputs ...string) []track {
	t.Helper()
	tracks, err := validateInput(playlistInput{Title: "Title", Tracks: inputs})
	if err != nil {
		t.Fatal(err)
	}
	return tracks
}

func TestResolveTracks(t *testing.T) {
	calls := searchStub(t)
	tracks := searchInputs(t, "VIDEO_ID_01", "first song", "second song", " Artist, Song & live ")
	if err := resolveTracks(context.Background(), dependencies{searchTimeout: time.Second}, tracks); err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"VIDEO_ID_01", "VIDEO_ID_02", "VIDEO_ID_03", "VIDEO_ID_04"} {
		if tracks[i].VideoID == nil || *tracks[i].VideoID != id || tracks[i].Title != nil || tracks[i].Channel != nil || tracks[i].Error != "" {
			t.Fatalf("track %d = %#v", i, tracks[i])
		}
	}
	data, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "call\narg:--ignore-config\narg:--flat-playlist\narg:--playlist-items\narg:1\narg:--no-warnings\narg:--quiet\narg:--print\narg:%(id)s\t%(title)s\narg:https://music.youtube.com/search?q="
	want := prefix + "first+song#songs\ncredential:absent\n" + prefix + "second+song#songs\ncredential:absent\n" + prefix + "+Artist%2C+Song+%26+live+#songs\ncredential:absent\n"
	if string(data) != want {
		t.Fatalf("invocations = %q, want %q", data, want)
	}
}

func TestMissingSearchDependency(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if err := resolveTracks(context.Background(), dependencies{}, searchInputs(t, "VIDEO_ID_01")); err != nil {
		t.Fatalf("exact tracks need yt-dlp: %v", err)
	}
	err := resolveTracks(context.Background(), dependencies{}, searchInputs(t, "VIDEO_ID_01", "first song"))
	if err == nil || errors.Is(err, errRejected) || !strings.Contains(err.Error(), "brew install yt-dlp") {
		t.Fatalf("missing binary error = %v", err)
	}
}

func TestResolveEveryQueryBeforeRejection(t *testing.T) {
	calls := searchStub(t)
	inputs := []string{"empty result", "first song", "failed result", "timeout result", "second song", "VIDEO_ID_01"}
	tracks := searchInputs(t, inputs...)
	err := resolveTracks(context.Background(), dependencies{searchTimeout: time.Second}, tracks)
	if !errors.Is(err, errRejected) {
		t.Fatalf("error = %v", err)
	}
	for i, entry := range tracks {
		failed := i == 0 || i == 2 || i == 3
		if failed != (entry.Error != "") || failed != (entry.VideoID == nil) || entry.Title != nil || entry.Channel != nil {
			t.Errorf("track %d = %#v", i, entry)
		}
		if entry.Input != inputs[i] || strings.Contains(entry.Error, "SECRET") {
			t.Errorf("track data changed or leaked stderr: %#v", entry)
		}
	}
	data, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "call\n") != 5 {
		t.Fatalf("calls = %q", data)
	}
}

func TestMalformedSearchResults(t *testing.T) {
	cases := []struct{ query, message string }{
		{"bad id", "invalid result"},
		{"two results", "invalid result"},
		{"missing title", "invalid result"},
		{"missing tab", "invalid result"},
		{"invalid utf8", "invalid result"},
		{"large result", "exceeds 65536 bytes"},
		{"empty result", "no result"},
		{"failed result", "did not resolve"},
		{"timeout result", "timed out"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			searchStub(t)
			tracks := searchInputs(t, tc.query)
			err := resolveTracks(context.Background(), dependencies{searchTimeout: time.Second}, tracks)
			if !errors.Is(err, errRejected) || !strings.Contains(tracks[0].Error, tc.message) || tracks[0].VideoID != nil {
				t.Fatalf("track = %#v, error = %v", tracks[0], err)
			}
		})
	}
}

func TestDuplicateNormalizedTracks(t *testing.T) {
	cases := []struct {
		name   string
		inputs []string
	}{
		{name: "exact", inputs: []string{"VIDEO_ID_01", "https://youtu.be/VIDEO_ID_01"}},
		{name: "queries", inputs: []string{"second song", "duplicate song"}},
		{name: "exact and query", inputs: []string{"VIDEO_ID_03", "second song", "duplicate song"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			searchStub(t)
			tracks := searchInputs(t, tc.inputs...)
			if err := resolveTracks(context.Background(), dependencies{searchTimeout: time.Second}, tracks); !errors.Is(err, errRejected) {
				t.Fatalf("error = %v", err)
			}
			for i, entry := range tracks {
				if entry.Error == "" || entry.VideoID == nil || entry.Title != nil || entry.Channel != nil {
					t.Fatalf("track %d = %#v", i, entry)
				}
				if entry.Source == "search" && (!strings.Contains(entry.Error, entry.Input) || !strings.Contains(entry.Error, *entry.VideoID)) {
					t.Errorf("missing query resolution in error: %#v", entry)
				}
			}
			if !strings.Contains(tracks[0].Error, "position(s) 2") || !strings.Contains(tracks[1].Error, "position(s) 1") {
				t.Fatalf("duplicate positions = %#v", tracks)
			}
		})
	}
}

func TestSearchCancellation(t *testing.T) {
	calls := searchStub(t)
	tracks := searchInputs(t, "timeout result", "second song")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	finished := make(chan error, 1)
	go func() {
		finished <- resolveTracks(ctx, dependencies{searchTimeout: time.Minute}, tracks)
	}()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
waitForStart:
	for {
		select {
		case <-deadline.C:
			t.Fatal("search stub did not start")
		case <-tick.C:
			if data, err := os.ReadFile(calls); err == nil && strings.Contains(string(data), "credential:absent\n") {
				break waitForStart
			}
		}
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) || errors.Is(err, errRejected) {
			t.Fatalf("cancellation error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("search cancellation was not prompt")
	}
	data, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "call\n") != 1 || tracks[1].VideoID != nil {
		t.Fatalf("search continued after cancellation: %q, %#v", data, tracks)
	}
	alreadyCanceled, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	before := append([]byte(nil), data...)
	if err := resolveTracks(alreadyCanceled, dependencies{}, tracks); !errors.Is(err, context.Canceled) {
		t.Fatalf("already canceled error = %v", err)
	}
	data, err = os.ReadFile(calls)
	if err != nil || !reflect.DeepEqual(data, before) {
		t.Fatalf("already canceled search ran: %q, %v", data, err)
	}
}
