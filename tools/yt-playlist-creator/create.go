package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

type createResult struct {
	Status         string  `json:"status"`
	PlaylistID     string  `json:"playlist_id,omitempty"`
	URL            string  `json:"url,omitempty"`
	Privacy        string  `json:"privacy,omitempty"`
	Title          *string `json:"title,omitempty"`
	Description    *string `json:"description,omitempty"`
	Added          *int    `json:"added,omitempty"`
	NextIndex      *int    `json:"next_index,omitempty"`
	OutcomeUnknown *bool   `json:"outcome_unknown,omitempty"`
	Error          string  `json:"error,omitempty"`
	Tracks         []track `json:"tracks,omitempty"`
}

func newCreateCommand(deps dependencies) *cobra.Command {
	var input playlistInput
	var file, output string
	var dryRun bool
	cmd := &cobra.Command{
		Use: "create", Short: "Create one private playlist from an ordered track list",
		Long: "Create one private YouTube playlist that can be opened in YouTube Music.\n\n" +
			"Use title/description/track flags or one JSON object from --file (use - for stdin).\n" +
			"Tracks are exact video IDs, HTTPS watch/youtu.be links, or song queries.\n" +
			"Queries use the first anonymous yt-dlp Songs result. No pages are fetched.\n" +
			"An 11-character word such as Bad_Romance is treated as a video ID.\n\n" +
			"Use --dry-run to resolve and validate without writing. Credentials are required.\n" +
			"Use --output json for one status object, including errors after flag parsing.\n" +
			"Each invocation creates a new playlist. Never blindly retry partial or unknown results.\n" +
			"YouTube Music availability and playback are not guaranteed.",
		RunE: func(cmd *cobra.Command, args []string) error {
			var result createResult
			var err error
			switch {
			case len(args) != 0:
				err = fmt.Errorf("create accepts no positional arguments")
			case output != "url" && output != "json":
				err = fmt.Errorf("--output must be url or json")
			default:
				flagsSet := cmd.Flags().Changed("title") || cmd.Flags().Changed("description") || cmd.Flags().Changed("track")
				input, err = readInput(cmd.Context(), input, file, cmd.Flags().Changed("file"), flagsSet, deps.stdin)
				if err == nil {
					result, err = createPlaylist(cmd.Context(), deps, input, dryRun)
				}
			}
			if err != nil && result.Status == "" {
				result = createResult{Status: "error", Error: err.Error()}
			}
			var writeErr error
			if output == "json" {
				if writeErr = json.NewEncoder(deps.stdout).Encode(result); writeErr != nil {
					writeErr = fmt.Errorf("write JSON result: %w", writeErr)
				}
			} else if err == nil {
				if result.Status == "created" {
					_, writeErr = fmt.Fprintln(deps.stdout, result.URL)
				} else {
					_, writeErr = fmt.Fprint(deps.stdout, preview(input, result.Tracks))
				}
			}
			if result.Status == "partial" || result.Status == "unknown" || result.Status == "created" && writeErr != nil {
				err = errors.New(recoveryMessage(result))
			}
			if err != nil && result.Status == "rejected" {
				var message bytes.Buffer
				message.WriteString(result.Error)
				for _, entry := range result.Tracks {
					if entry.Error != "" {
						fmt.Fprintf(&message, "\n%s", entry.Error)
					}
				}
				err = errors.New(message.String())
			}
			return errors.Join(err, writeErr)
		},
	}
	cmd.Flags().StringVar(&input.Title, "title", "", "playlist title (required with --track)")
	cmd.Flags().StringVar(&input.Description, "description", "", "playlist description")
	cmd.Flags().StringArrayVar(&input.Tracks, "track", nil, "video link, ID, or song query; repeat in order (1 to 100)")
	cmd.Flags().StringVar(&file, "file", "", "JSON input file; - reads stdin (maximum 1 MiB)")
	cmd.Flags().StringVar(&output, "output", "url", "output format: url or json")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "resolve and validate tracks without playlist writes")
	return cmd
}

func createPlaylist(ctx context.Context, deps dependencies, input playlistInput, dryRun bool) (createResult, error) {
	tracks, err := validateInput(input)
	if err == nil {
		err = resolveTracks(ctx, deps, tracks)
	}
	if err != nil {
		return preflightFailure(err, tracks), err
	}
	bundle, err := loadCredentials(ctx, deps)
	if err != nil {
		return preflightFailure(err, nil), err
	}
	client := newYouTubeClient(ctx, deps, bundle)
	if err := client.preflight(ctx, tracks); err != nil {
		return preflightFailure(err, tracks), err
	}
	if err := ctx.Err(); err != nil {
		return preflightFailure(err, nil), err
	}
	if dryRun {
		return createResult{Status: "validated", Title: &input.Title, Description: &input.Description, Privacy: "private", Tracks: tracks}, nil
	}
	playlistID, err := client.createPlaylist(ctx, input)
	if err != nil {
		if playlistID != "" {
			return partialResult(playlistID, tracks, 0, err), err
		}
		if outcomeUnknown(err) {
			unknown := true
			message := err.Error() + "; a playlist may already exist; inspect your YouTube account before retrying"
			return createResult{Status: "unknown", OutcomeUnknown: &unknown, Error: message}, errors.New(message)
		}
		return preflightFailure(err, nil), err
	}
	for index, entry := range tracks {
		if err := client.insertTrack(ctx, playlistID, *entry.VideoID); err != nil {
			return partialResult(playlistID, tracks, index, err), err
		}
	}
	added := len(tracks)
	return createResult{Status: "created", PlaylistID: playlistID, URL: playlistURL(playlistID), Privacy: "private", Added: &added, Tracks: tracks}, nil
}

func preflightFailure(err error, tracks []track) createResult {
	result := createResult{Status: "error", Error: err.Error()}
	if errors.Is(err, errRejected) {
		result.Status = "rejected"
		result.Tracks = tracks
	}
	return result
}

func partialResult(playlistID string, tracks []track, added int, err error) createResult {
	unknown := outcomeUnknown(err)
	return createResult{
		Status: "partial", PlaylistID: playlistID, URL: playlistURL(playlistID),
		Added: &added, NextIndex: &added, OutcomeUnknown: &unknown, Error: err.Error(), Tracks: tracks,
	}
}

func playlistURL(id string) string {
	return "https://music.youtube.com/playlist?list=" + url.QueryEscape(id)
}

func recoveryMessage(result createResult) string {
	if result.Status == "unknown" {
		return result.Error
	}
	data, _ := json.Marshal(result.Tracks)
	if result.Status == "created" {
		return fmt.Sprintf("Playlist creation completed, but the result could not be written.\nPlaylist: %s\nConfirmed added: %d\nTracks: %s\nDo not rerun create; it creates another playlist.", result.URL, *result.Added, data)
	}
	return fmt.Sprintf("%s\nPlaylist: %s\nConfirmed added: %d; next_index: %d; outcome_unknown: %t\nTracks: %s\nInspect the playlist before manual recovery. Do not rerun create; it creates another playlist.",
		result.Error, result.URL, *result.Added, *result.NextIndex, *result.OutcomeUnknown, data)
}

func preview(input playlistInput, tracks []track) string {
	var output bytes.Buffer
	fmt.Fprintf(&output, "Validated private playlist: %q\nDescription: %q\n", input.Title, input.Description)
	for index, entry := range tracks {
		fmt.Fprintf(&output, "%d. %q -> %s | %q | %q (%s)\n", index+1, entry.Input, *entry.VideoID, *entry.Title, *entry.Channel, entry.Source)
	}
	output.WriteString("No playlist was created. Later insertion and YouTube Music playback are not guaranteed.\n")
	return output.String()
}
