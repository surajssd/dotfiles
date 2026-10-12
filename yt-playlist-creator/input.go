package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"unicode/utf8"
)

const maxInputBytes = 1 << 20

var errRejected = errors.New("one or more tracks were rejected")

type playlistInput struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tracks      []string `json:"tracks"`
}

type track struct {
	Input   string  `json:"input"`
	VideoID *string `json:"video_id"`
	Title   *string `json:"title"`
	Channel *string `json:"channel"`
	Source  string  `json:"source"`
	Error   string  `json:"error,omitempty"`
}

func readInput(ctx context.Context, input playlistInput, file string, fileSet, inputFlagsSet bool, stdin io.Reader) (playlistInput, error) {
	if err := ctx.Err(); err != nil {
		return playlistInput{}, err
	}
	if fileSet && inputFlagsSet {
		return playlistInput{}, fmt.Errorf("--file cannot be combined with --title, --description, or --track")
	}
	if !fileSet {
		if !inputFlagsSet {
			return playlistInput{}, fmt.Errorf("provide --file or --title and --track; stdin is read only with --file -")
		}
		if err := inputUTF8(input); err != nil {
			return playlistInput{}, err
		}
		data, err := json.Marshal(input)
		if err != nil {
			return playlistInput{}, fmt.Errorf("encode playlist input: %w", err)
		}
		if len(data) > maxInputBytes {
			return playlistInput{}, fmt.Errorf("canonical input exceeds %d bytes", maxInputBytes)
		}
		return input, nil
	}
	if file == "" {
		return playlistInput{}, fmt.Errorf("--file must not be empty")
	}
	reader := stdin
	if file != "-" {
		f, err := os.Open(file)
		if err != nil {
			return playlistInput{}, fmt.Errorf("open playlist input: %w", err)
		}
		defer func() { _ = f.Close() }()
		reader = f
	}
	data, err := readBounded(ctx, reader, maxInputBytes)
	if err != nil {
		return playlistInput{}, err
	}
	if !utf8.Valid(data) {
		return playlistInput{}, fmt.Errorf("input must be valid UTF-8")
	}
	return decodePlaylistInput(data)
}

// Cancellation returns before a blocked Read completes; its owner must release the reader.
func readBounded(ctx context.Context, reader io.Reader, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	type result struct {
		data []byte
		err  error
	}
	finished := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(io.LimitReader(reader, limit+1))
		finished <- result{data: data, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case read := <-finished:
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if read.err != nil {
			return nil, fmt.Errorf("read input: %w", read.err)
		}
		if int64(len(read.data)) > limit {
			return nil, fmt.Errorf("input exceeds %d bytes", limit)
		}
		return read.data, nil
	}
}

func decodePlaylistInput(data []byte) (playlistInput, error) {
	var input playlistInput
	decoder := json.NewDecoder(bytes.NewReader(data))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return input, fmt.Errorf("input must be one JSON object")
	}
	seen := make(map[string]bool)
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return input, fmt.Errorf("invalid JSON input")
		}
		name, ok := key.(string)
		if !ok {
			return input, fmt.Errorf("invalid JSON input")
		}
		switch name {
		case "title", "description", "tracks":
		default:
			return input, fmt.Errorf("input fields must be title, description, and tracks")
		}
		if seen[name] {
			return input, fmt.Errorf("input repeats the %s field", name)
		}
		seen[name] = true
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return input, fmt.Errorf("invalid JSON input")
		}
		if name == "tracks" {
			var entries []json.RawMessage
			if len(value) == 0 || value[0] != '[' || json.Unmarshal(value, &entries) != nil {
				return input, fmt.Errorf("tracks must be an array of strings")
			}
			input.Tracks = make([]string, len(entries))
			for i, entry := range entries {
				if len(entry) == 0 || entry[0] != '"' || json.Unmarshal(entry, &input.Tracks[i]) != nil {
					return input, fmt.Errorf("track at input position %d must be a string", i+1)
				}
			}
			continue
		}
		var text string
		if len(value) == 0 || value[0] != '"' || json.Unmarshal(value, &text) != nil {
			return input, fmt.Errorf("%s must be a string", name)
		}
		if name == "title" {
			input.Title = text
		} else {
			input.Description = text
		}
	}
	if end, err := decoder.Token(); err != nil || end != json.Delim('}') {
		return input, fmt.Errorf("invalid JSON input")
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return input, fmt.Errorf("input must contain exactly one JSON object")
	}
	return input, nil
}

func inputUTF8(input playlistInput) error {
	if !utf8.ValidString(input.Title) || !utf8.ValidString(input.Description) {
		return fmt.Errorf("title and description must be valid UTF-8")
	}
	for i, text := range input.Tracks {
		if !utf8.ValidString(text) {
			return fmt.Errorf("track at input position %d must be valid UTF-8", i+1)
		}
	}
	return nil
}

func validateInput(input playlistInput) ([]track, error) {
	if err := inputUTF8(input); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Title) == "" {
		return nil, fmt.Errorf("title must not be blank")
	}
	if len(input.Tracks) < 1 || len(input.Tracks) > 100 {
		return nil, fmt.Errorf("provide between 1 and 100 tracks")
	}
	tracks := make([]track, len(input.Tracks))
	rejected := false
	for i, text := range input.Tracks {
		entry := track{Input: text, Source: "search"}
		switch {
		case strings.TrimSpace(text) == "":
			entry.Error = "track must not be blank"
		case isVideoID(text):
			entry.Source = "exact"
			entry.VideoID = &text
		case strings.HasPrefix(strings.ToLower(text), "http://"), strings.HasPrefix(strings.ToLower(text), "https://"):
			entry.Source = "exact"
			id, err := videoURL(text)
			if err != nil {
				entry.Error = err.Error()
			} else {
				entry.VideoID = &id
			}
		}
		if entry.Error != "" {
			rejected = true
		}
		tracks[i] = entry
	}
	if rejected {
		for i := range tracks {
			tracks[i].VideoID = nil
		}
		return tracks, errRejected
	}
	return tracks, nil
}

func isVideoID(value string) bool {
	if len(value) != 11 {
		return false
	}
	for i := range len(value) {
		c := value[i]
		valid := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
		if !valid {
			return false
		}
	}
	return true
}

func videoURL(text string) (string, error) {
	invalid := fmt.Errorf("expected an HTTPS YouTube watch link or youtu.be video link with a valid video ID")
	parsed, err := url.Parse(text)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Opaque != "" {
		return "", invalid
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "", invalid
	}
	switch strings.ToLower(parsed.Host) {
	case "music.youtube.com", "youtube.com", "www.youtube.com", "m.youtube.com":
		if parsed.EscapedPath() != "/watch" || len(query["v"]) != 1 || !isVideoID(query.Get("v")) {
			return "", invalid
		}
		return query.Get("v"), nil
	case "youtu.be":
		id := strings.TrimPrefix(parsed.EscapedPath(), "/")
		if !strings.HasPrefix(parsed.EscapedPath(), "/") || !isVideoID(id) {
			return "", invalid
		}
		return id, nil
	default:
		return "", invalid
	}
}
