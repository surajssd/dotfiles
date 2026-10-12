package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"
)

const maxSearchOutput = 64 << 10

type searchOutput struct {
	data     []byte
	tooLarge bool
}

func (output *searchOutput) Write(data []byte) (int, error) {
	available := maxSearchOutput - len(output.data)
	if len(data) > available {
		output.data = append(output.data, data[:available]...)
		output.tooLarge = true
	} else {
		output.data = append(output.data, data...)
	}
	return len(data), nil
}

func resolveTracks(ctx context.Context, deps dependencies, tracks []track) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var binary string
	for _, entry := range tracks {
		if entry.Source == "search" {
			var err error
			binary, err = exec.LookPath("yt-dlp")
			if err != nil {
				return fmt.Errorf("yt-dlp is required for song queries; install it with brew install yt-dlp")
			}
			break
		}
	}
	for i := range tracks {
		if err := ctx.Err(); err != nil {
			return err
		}
		if tracks[i].Source != "search" {
			continue
		}
		id, err := resolveQuery(ctx, deps.searchTimeout, binary, tracks[i].Input)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			tracks[i].Error = err.Error()
			continue
		}
		tracks[i].VideoID = &id
	}
	positions := make(map[string][]int)
	for i, entry := range tracks {
		if entry.VideoID != nil {
			positions[*entry.VideoID] = append(positions[*entry.VideoID], i)
		}
	}
	for i := range tracks {
		entry := &tracks[i]
		if entry.VideoID == nil || len(positions[*entry.VideoID]) < 2 {
			continue
		}
		var others []string
		for _, position := range positions[*entry.VideoID] {
			if position != i {
				others = append(others, fmt.Sprint(position+1))
			}
		}
		entry.Error = fmt.Sprintf("duplicate video ID %q also used by input position(s) %s", *entry.VideoID, strings.Join(others, ", "))
		if entry.Source == "search" {
			entry.Error = fmt.Sprintf("query %q resolved to %q, also used by input position(s) %s", entry.Input, *entry.VideoID, strings.Join(others, ", "))
		}
	}
	for _, entry := range tracks {
		if entry.Error != "" {
			return errRejected
		}
	}
	return nil
}

func resolveQuery(ctx context.Context, timeout time.Duration, binary, query string) (string, error) {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	searchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	searchURL := "https://music.youtube.com/search?q=" + url.QueryEscape(query) + "#songs"
	command := exec.CommandContext(searchCtx, binary,
		"--ignore-config", "--flat-playlist", "--playlist-items", "1", "--no-warnings", "--quiet",
		"--print", "%(id)s\t%(title)s", searchURL)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "YT_PLAYLIST_CREATOR_OAUTH=") {
			command.Env = append(command.Env, value)
		}
	}
	var output searchOutput
	command.Stdout = &output
	command.Stderr = io.Discard
	command.WaitDelay = time.Second
	err := command.Run()
	if searchCtx.Err() != nil {
		return "", fmt.Errorf("song search timed out or was canceled: %w", searchCtx.Err())
	}
	if err != nil {
		return "", fmt.Errorf("yt-dlp did not resolve this query successfully")
	}
	if output.tooLarge {
		return "", fmt.Errorf("song search output exceeds %d bytes", maxSearchOutput)
	}
	text := strings.TrimSuffix(string(output.data), "\n")
	text = strings.TrimSuffix(text, "\r")
	if text == "" {
		return "", fmt.Errorf("song search returned no result")
	}
	id, title, found := strings.Cut(text, "\t")
	if !found || !isVideoID(id) || strings.TrimSpace(title) == "" || strings.ContainsAny(text, "\r\n") || !utf8.ValidString(text) {
		return "", fmt.Errorf("song search returned an invalid result")
	}
	return id, nil
}
