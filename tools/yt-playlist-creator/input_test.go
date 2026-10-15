package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestReadInputModes(t *testing.T) {
	want := playlistInput{
		Title: "  Evening 世界  ", Description: "Original\nsource", Tracks: []string{"VIDEO_ID_01", "A song, An artist"},
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "playlist.json")
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, file string
		input      playlistInput
		fileSet    bool
		flagsSet   bool
		stdin      io.Reader
	}{
		{name: "flags", input: want, flagsSet: true, stdin: unreadableInput{}},
		{name: "file", file: file, fileSet: true, stdin: unreadableInput{}},
		{name: "stdin", file: "-", fileSet: true, stdin: strings.NewReader(string(data))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readInput(context.Background(), tc.input, tc.file, tc.fileSet, tc.flagsSet, tc.stdin)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("input = %#v, error = %v, want %#v", got, err, want)
			}
		})
	}
}

type unreadableInput struct{}

func (unreadableInput) Read([]byte) (int, error) {
	panic("unexpected input read")
}

func TestReadInputModeErrors(t *testing.T) {
	cases := []struct {
		name, file, want string
		fileSet          bool
		flagsSet         bool
	}{
		{name: "missing", want: "provide --file"},
		{name: "conflicting empty flag", file: "-", fileSet: true, flagsSet: true, want: "cannot be combined"},
		{name: "empty file", fileSet: true, want: "must not be empty"},
		{name: "missing file", file: filepath.Join(t.TempDir(), "missing.json"), fileSet: true, want: "open playlist input"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := readInput(context.Background(), playlistInput{}, tc.file, tc.fileSet, tc.flagsSet, unreadableInput{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestStrictPlaylistJSON(t *testing.T) {
	cases := []struct{ name, input, want string }{
		{name: "empty", want: "one JSON object"},
		{name: "null", input: "null", want: "one JSON object"},
		{name: "array", input: "[]", want: "one JSON object"},
		{name: "string", input: `"SECRET"`, want: "one JSON object"},
		{name: "incomplete", input: `{"title":`, want: "invalid JSON"},
		{name: "trailing object", input: `{"title":"A"} {}`, want: "exactly one JSON object"},
		{name: "trailing invalid data", input: `{"title":"A"} SECRET`, want: "exactly one JSON object"},
		{name: "unknown field", input: `{"SECRET":"value"}`, want: "input fields"},
		{name: "wrong case", input: `{"Title":"A"}`, want: "input fields"},
		{name: "duplicate field", input: `{"title":"A","title":"B"}`, want: "repeats the title"},
		{name: "null title", input: `{"title":null}`, want: "title must be a string"},
		{name: "number title", input: `{"title":123}`, want: "title must be a string"},
		{name: "null description", input: `{"description":null}`, want: "description must be a string"},
		{name: "object description", input: `{"description":{}}`, want: "description must be a string"},
		{name: "null tracks", input: `{"tracks":null}`, want: "array of strings"},
		{name: "string tracks", input: `{"tracks":"SECRET"}`, want: "array of strings"},
		{name: "object tracks", input: `{"tracks":{}}`, want: "array of strings"},
		{name: "null track", input: `{"tracks":["VIDEO_ID_01",null]}`, want: "position 2 must be a string"},
		{name: "number track", input: `{"tracks":[3]}`, want: "position 1 must be a string"},
		{name: "nested tracks", input: `{"tracks":[["VIDEO_ID_01"]]}`, want: "position 1 must be a string"},
		{name: "invalid UTF8", input: "{\"title\":\"\xff\"}", want: "UTF-8"},
		{name: "oversized", input: strings.Repeat(" ", maxInputBytes+1), want: "exceeds 1048576 bytes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := readInput(context.Background(), playlistInput{}, "-", true, false, strings.NewReader(tc.input))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if strings.Contains(err.Error(), "SECRET") {
				t.Fatalf("error includes raw input: %v", err)
			}
		})
	}
	got, err := readInput(context.Background(), playlistInput{}, "-", true, false, strings.NewReader("{\"title\": \"A\", \"tracks\": [ \"VIDEO_ID_01\" ]}\n\t"))
	if err != nil || got.Description != "" || !reflect.DeepEqual(got.Tracks, []string{"VIDEO_ID_01"}) {
		t.Fatalf("input = %#v, error = %v", got, err)
	}
}

func TestCanonicalFlagInputBound(t *testing.T) {
	cases := []struct {
		name  string
		input playlistInput
	}{
		{name: "title bytes", input: playlistInput{Title: strings.Repeat("a", maxInputBytes)}},
		{name: "escaped title bytes", input: playlistInput{Title: strings.Repeat("\n", maxInputBytes/2)}},
		{name: "invalid title UTF8", input: playlistInput{Title: "\xff"}},
		{name: "invalid description UTF8", input: playlistInput{Title: "A", Description: "\xff"}},
		{name: "invalid track UTF8", input: playlistInput{Title: "A", Tracks: []string{"\xff"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := readInput(context.Background(), tc.input, "", false, true, unreadableInput{}); err == nil {
				t.Fatal("invalid input accepted")
			}
		})
	}
	input := playlistInput{Title: "A", Tracks: []string{"VIDEO_ID_01"}}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Description = strings.Repeat("a", maxInputBytes-len(data))
	if _, err := readInput(context.Background(), input, "", false, true, unreadableInput{}); err != nil {
		t.Fatalf("exact size bound rejected: %v", err)
	}
}

func TestReadBoundedCancellation(t *testing.T) {
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = reader.Close() })
	t.Cleanup(func() { _ = writer.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	finished := make(chan error, 1)
	go func() {
		_, err := readBounded(ctx, reader, maxInputBytes)
		finished <- err
	}()
	if _, err := writer.Write([]byte("{")); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not interrupt stdin")
	}
	if _, err := readBounded(ctx, unreadableInput{}, maxInputBytes); !errors.Is(err, context.Canceled) {
		t.Fatalf("already canceled read = %v", err)
	}
}

func TestTrackClassification(t *testing.T) {
	cases := []struct{ input, source, id string }{
		{input: "VIDEO_ID_01", source: "exact", id: "VIDEO_ID_01"},
		{input: "a_0-9AbCdEf", source: "exact", id: "a_0-9AbCdEf"},
		{input: "Bad_Romance", source: "exact", id: "Bad_Romance"},
		{input: "https://music.youtube.com/watch?v=VIDEO_ID_01", source: "exact", id: "VIDEO_ID_01"},
		{input: "https://youtube.com/watch?v=VIDEO_ID_01&list=playlist&feature=share#part", source: "exact", id: "VIDEO_ID_01"},
		{input: "https://www.youtube.com/watch?v=VIDEO_ID_01", source: "exact", id: "VIDEO_ID_01"},
		{input: "https://m.youtube.com/watch?v=VIDEO_ID_01", source: "exact", id: "VIDEO_ID_01"},
		{input: "https://MUSIC.YOUTUBE.COM/watch?v=VIDEO_ID_01", source: "exact", id: "VIDEO_ID_01"},
		{input: "https://youtu.be/VIDEO_ID_01?si=tracking#fragment", source: "exact", id: "VIDEO_ID_01"},
		{input: "A song, An artist", source: "search"},
		{input: " Song   Artist ", source: "search"},
		{input: "123", source: "search"},
		{input: "世界の歌", source: "search"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			tracks, err := validateInput(playlistInput{Title: "Title", Tracks: []string{tc.input}})
			if err != nil || len(tracks) != 1 {
				t.Fatalf("tracks = %#v, error = %v", tracks, err)
			}
			entry := tracks[0]
			if entry.Input != tc.input || entry.Source != tc.source || entry.Error != "" || entry.Title != nil || entry.Channel != nil {
				t.Fatalf("track = %#v", entry)
			}
			if tc.id == "" && entry.VideoID != nil || tc.id != "" && (entry.VideoID == nil || *entry.VideoID != tc.id) {
				t.Fatalf("video ID = %v, want %q", entry.VideoID, tc.id)
			}
		})
	}
}

func TestLocalTrackRejections(t *testing.T) {
	invalid := []string{
		"", " \n\t", "http://youtube.com/watch?v=VIDEO_ID_01", "https://evil.example/watch?v=VIDEO_ID_01",
		"https://youtube.com:443/watch?v=VIDEO_ID_01", "https://youtube.com:/watch?v=VIDEO_ID_01",
		"https://user@youtube.com/watch?v=VIDEO_ID_01", "https://youtube.com.evil.example/watch?v=VIDEO_ID_01",
		"https://youtube.com/shorts/VIDEO_ID_01", "https://youtube.com/embed/VIDEO_ID_01", "https://youtube.com/%77atch?v=VIDEO_ID_01",
		"https://youtube.com/watch", "https://youtube.com/watch?v=", "https://youtube.com/watch?v=short",
		"https://youtube.com/watch?v=VIDEO_ID_01&v=VIDEO_ID_02", "https://youtube.com/watch?v=VIDEO_ID_01&v=",
		"https://youtube.com/playlist?list=playlist", "https://youtube.com/watch?list=playlist",
		"https://youtube.com/watch?v=VIDEO_ID_01;v=VIDEO_ID_02", "https://youtube.com/watch?v=%FF",
		"https://youtu.be/VIDEO_ID_01/", "https://youtu.be/", "https://youtu.be/VIDEO_ID_01/other",
		"https://youtu.be/%56IDEO_ID_01", "https://youtube.com/watch?v=VIDEO_ID_01 ", "https://[bad",
	}
	for _, text := range invalid {
		t.Run(text, func(t *testing.T) {
			tracks, err := validateInput(playlistInput{Title: "Title", Tracks: []string{"VIDEO_ID_01", text, "Query for a song"}})
			if !errors.Is(err, errRejected) || len(tracks) != 3 {
				t.Fatalf("tracks = %#v, error = %v", tracks, err)
			}
			for i, entry := range tracks {
				if entry.VideoID != nil || entry.Title != nil || entry.Channel != nil || (entry.Error != "") != (i == 1) {
					t.Errorf("track %d = %#v", i, entry)
				}
			}
			if strings.HasPrefix(text, "http") && tracks[1].Source != "exact" {
				t.Fatalf("invalid URL source = %q", tracks[1].Source)
			}
		})
	}
	tracks, err := validateInput(playlistInput{Title: "Title", Tracks: []string{"", "VIDEO_ID_01", "https://evil.example"}})
	if !errors.Is(err, errRejected) || tracks[0].Error == "" || tracks[1].Error != "" || tracks[2].Error == "" {
		t.Fatalf("not all invalid tracks reported: %#v, %v", tracks, err)
	}
}

func TestInputShapeValidation(t *testing.T) {
	cases := []playlistInput{
		{}, {Title: " \n\t", Tracks: []string{"VIDEO_ID_01"}}, {Title: "Title"},
		{Title: "Title", Tracks: make([]string, 101)},
	}
	for _, input := range cases {
		tracks, err := validateInput(input)
		if err == nil || errors.Is(err, errRejected) || tracks != nil {
			t.Fatalf("input shape = %#v, tracks = %#v, error = %v", input, tracks, err)
		}
	}
	input := playlistInput{Title: "Title", Tracks: make([]string, 100)}
	for i := range input.Tracks {
		input.Tracks[i] = "VIDEO_ID_01"
	}
	if _, err := validateInput(input); err != nil {
		t.Fatalf("100 tracks rejected before resolution: %v", err)
	}
}
