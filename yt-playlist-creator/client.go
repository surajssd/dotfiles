package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/oauth2"
)

type youtubeClient struct {
	baseURL string
	http    *http.Client
	tokens  oauth2.TokenSource
}

func boundedHTTPClient(base *http.Client) *http.Client {
	client := *base
	if client.Timeout <= 0 || client.Timeout > 30*time.Second {
		client.Timeout = 30 * time.Second
	}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	client.Jar = nil
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	if transport, ok := transport.(*http.Transport); ok {
		isolated := transport.Clone()
		isolated.DisableKeepAlives = true
		client.Transport = isolated
	}
	return &client
}

func newYouTubeClient(ctx context.Context, deps dependencies, bundle credentials) *youtubeClient {
	config := oauthConfig(deps, bundle)
	return &youtubeClient{
		baseURL: deps.apiBaseURL, http: boundedHTTPClient(deps.httpClient),
		tokens: config.TokenSource(oauthContext(ctx, deps), &oauth2.Token{RefreshToken: bundle.RefreshToken}),
	}
}

type apiFailure struct {
	message        string
	outcomeUnknown bool
}

func (e *apiFailure) Error() string { return e.message }

func outcomeUnknown(err error) bool {
	var failure *apiFailure
	return errors.As(err, &failure) && failure.outcomeUnknown
}

func (c *youtubeClient) request(ctx context.Context, method, path string, body, result any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	token, err := c.tokens.Token()
	if err != nil {
		return oauthError(ctx, err)
	}
	var payload []byte
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("cannot encode the YouTube request")
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("cannot prepare the YouTube request")
	}
	// Non-replayable bodies and fresh connections prevent implicit transport retries.
	req.GetBody = nil
	req.Close = true
	token.SetAuthHeader(req)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	mutating := method != http.MethodGet
	resp, err := c.http.Do(req)
	if err != nil {
		message := "YouTube request failed before a response was received"
		if ctx.Err() != nil {
			message = "YouTube request interrupted: " + ctx.Err().Error()
		} else if errors.Is(err, context.DeadlineExceeded) {
			message = "YouTube request timed out"
		}
		return &apiFailure{message: message, outcomeUnknown: mutating}
	}
	defer func() { _ = resp.Body.Close() }()
	const responseLimit = 4 << 20
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, responseLimit+1))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &apiFailure{
			message:        apiStatusMessage(resp, data),
			outcomeUnknown: mutating && (resp.StatusCode >= 500 || resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode >= 300 && resp.StatusCode < 400),
		}
	}
	if readErr != nil || len(data) > responseLimit || !utf8.Valid(data) || json.Unmarshal(data, result) != nil {
		return &apiFailure{message: "YouTube returned an unreadable success response", outcomeUnknown: mutating}
	}
	return nil
}

func apiStatusMessage(resp *http.Response, data []byte) string {
	message := "request was rejected"
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		message = "authorization expired or was revoked; run yt-playlist-creator auth login again"
	case http.StatusForbidden:
		message = "permission denied; check the authorized YouTube account and scope"
	case http.StatusNotFound:
		message = "the requested video, channel, or playlist is unavailable"
	case http.StatusTooManyRequests:
		message = "rate limit exceeded"
	default:
		if resp.StatusCode >= 500 {
			message = "YouTube service failed"
		} else if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			message = "HTTP redirects are not allowed"
		}
	}
	var envelope struct {
		Error struct {
			Errors []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &envelope) == nil {
		for _, detail := range envelope.Error.Errors {
			switch detail.Reason {
			case "authError", "invalidCredentials", "authorizationRequired":
				message = "authorization expired or was revoked; run yt-playlist-creator auth login again"
			case "insufficientPermissions", "insufficientAuthenticationScopes":
				message = "authorization lacks the YouTube scope; run yt-playlist-creator auth login again"
			case "youtubeSignupRequired", "channelNotFound":
				message = "the authorized account needs a YouTube channel; create a channel in YouTube and run auth login again"
			case "videoNotFound", "videoUnavailable", "invalidVideoId":
				message = "the video is missing or unavailable to this account"
			case "quotaExceeded", "dailyLimitExceeded", "dailyLimitExceededUnreg":
				message = "YouTube API quota is exhausted"
			case "maxPlaylistExceeded":
				message = "the account's playlist creation limit is exhausted"
			case "rateLimitExceeded", "userRateLimitExceeded":
				message = "rate limit exceeded"
			case "forbidden", "playlistForbidden", "playlistItemsNotAccessible":
				message = "permission denied for this account or playlist"
			}
		}
	}
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" && len(retryAfter) <= 128 {
		_, numberErr := strconv.ParseUint(retryAfter, 10, 64)
		_, dateErr := http.ParseTime(retryAfter)
		if numberErr == nil || dateErr == nil {
			message += "; Retry-After: " + retryAfter
		}
	}
	return fmt.Sprintf("youtube HTTP %d: %s", resp.StatusCode, message)
}

type videoSnippet struct {
	Title        *string `json:"title"`
	ChannelTitle *string `json:"channelTitle"`
}

func (c *youtubeClient) preflight(ctx context.Context, tracks []track) error {
	for start := 0; start < len(tracks); start += 50 {
		end := min(start+50, len(tracks))
		ids := make([]string, 0, end-start)
		for _, entry := range tracks[start:end] {
			ids = append(ids, *entry.VideoID)
		}
		query := url.Values{"part": {"snippet"}, "id": {strings.Join(ids, ",")}, "fields": {"items(id,snippet(title,channelTitle))"}}
		var result struct {
			Items []struct {
				ID      string        `json:"id"`
				Snippet *videoSnippet `json:"snippet"`
			} `json:"items"`
		}
		if err := c.request(ctx, http.MethodGet, "/videos?"+query.Encode(), nil, &result); err != nil {
			return err
		}
		if result.Items == nil {
			return fmt.Errorf("video response has no items array")
		}
		metadata := make(map[string]*videoSnippet, len(result.Items))
		for _, item := range result.Items {
			if !isVideoID(item.ID) || item.Snippet == nil || item.Snippet.Title == nil || item.Snippet.ChannelTitle == nil || metadata[item.ID] != nil {
				return fmt.Errorf("video response has invalid or repeated metadata")
			}
			metadata[item.ID] = item.Snippet
		}
		for index := start; index < end; index++ {
			entry := &tracks[index]
			if snippet := metadata[*entry.VideoID]; snippet != nil {
				entry.Title, entry.Channel = snippet.Title, snippet.ChannelTitle
			} else {
				entry.Error = fmt.Sprintf("track %d: video %s was not found or is unavailable", index+1, *entry.VideoID)
			}
		}
	}
	for _, entry := range tracks {
		if entry.Error != "" {
			return errRejected
		}
	}
	return nil
}

type playlistSnippet struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

type playlistStatus struct {
	Privacy string `json:"privacyStatus"`
}

type playlist struct {
	ID      string          `json:"id,omitempty"`
	Snippet playlistSnippet `json:"snippet"`
	Status  playlistStatus  `json:"status"`
}

func (c *youtubeClient) createPlaylist(ctx context.Context, input playlistInput) (string, error) {
	body := playlist{Snippet: playlistSnippet{Title: input.Title, Description: input.Description}, Status: playlistStatus{Privacy: "private"}}
	var result playlist
	if err := c.request(ctx, http.MethodPost, "/playlists?part=snippet,status", body, &result); err != nil {
		return "", err
	}
	if !isResourceID(result.ID) {
		return "", &apiFailure{message: "playlist response has no valid playlist ID", outcomeUnknown: true}
	}
	if result.Status.Privacy != "private" {
		return result.ID, &apiFailure{message: "playlist response did not confirm private visibility; inspect its visibility in YouTube", outcomeUnknown: true}
	}
	return result.ID, nil
}

type playlistItem struct {
	ID      string `json:"id,omitempty"`
	Snippet struct {
		PlaylistID string `json:"playlistId"`
		ResourceID struct {
			Kind    string `json:"kind"`
			VideoID string `json:"videoId"`
		} `json:"resourceId"`
	} `json:"snippet"`
}

func (c *youtubeClient) insertTrack(ctx context.Context, playlistID, videoID string) error {
	var body playlistItem
	body.Snippet.PlaylistID = playlistID
	body.Snippet.ResourceID.Kind = "youtube#video"
	body.Snippet.ResourceID.VideoID = videoID
	var result playlistItem
	if err := c.request(ctx, http.MethodPost, "/playlistItems?part=snippet", body, &result); err != nil {
		return err
	}
	if strings.TrimSpace(result.ID) == "" || result.Snippet.PlaylistID != playlistID || result.Snippet.ResourceID.Kind != "youtube#video" || result.Snippet.ResourceID.VideoID != videoID {
		return &apiFailure{message: "playlist item response did not confirm the requested insertion", outcomeUnknown: true}
	}
	return nil
}

func isResourceID(id string) bool {
	if len(id) == 0 || len(id) > 256 {
		return false
	}
	for _, c := range id {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' {
			continue
		}
		return false
	}
	return true
}
