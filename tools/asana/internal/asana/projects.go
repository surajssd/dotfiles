package asana

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (c *Client) CreateProject(ctx context.Context, name, workspace string) (string, error) {
	body, err := json.Marshal(map[string]any{"data": map[string]string{
		"name": name, "workspace": workspace, "privacy_setting": "private",
	}})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/projects?opt_fields=permalink_url,privacy_setting", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("prepare project request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("create project: %w; the project may exist; check Asana before retrying", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := apiStatusError(resp, "permission denied: check access to the workspace", "workspace is missing or inaccessible"); err != nil {
		return "", err
	}
	var result struct {
		Data struct {
			PermalinkURL   string `json:"permalink_url"`
			PrivacySetting string `json:"privacy_setting"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("read project response: %w; the project may exist; check Asana before retrying", err)
	}
	if _, err := ParseProjectURL(result.Data.PermalinkURL); err != nil {
		return "", fmt.Errorf("project response: %w; the project may exist; check Asana before retrying", err)
	}
	if result.Data.PrivacySetting != "private" {
		return "", fmt.Errorf("project response did not confirm private visibility (privacy_setting %q); check %s before retrying; the alias was not saved", result.Data.PrivacySetting, result.Data.PermalinkURL)
	}
	return result.Data.PermalinkURL, nil
}

type Project struct {
	GID          string `json:"gid"`
	Name         string `json:"name"`
	PermalinkURL string `json:"permalink_url"`
	Members      []struct {
		GID string `json:"gid"`
	} `json:"members"`
}

func (c *Client) FetchProjects(ctx context.Context, workspace string) ([]Project, error) {
	query := url.Values{
		"workspace": {workspace}, "archived": {"false"}, "limit": {"100"},
		"opt_fields": {"name,permalink_url,members.gid"},
	}
	var projects []Project
	offsets := make(map[string]bool)
	for {
		var result struct {
			Data     []Project `json:"data"`
			NextPage *struct {
				Offset string `json:"offset"`
			} `json:"next_page"`
		}
		if err := c.get(ctx, "/projects?"+query.Encode(), &result); err != nil {
			return nil, fmt.Errorf("list projects in workspace %s: %w", workspace, err)
		}
		if result.Data == nil {
			return nil, fmt.Errorf("response has no project data")
		}
		projects = append(projects, result.Data...)
		if result.NextPage == nil {
			return projects, nil
		}
		offset := result.NextPage.Offset
		if strings.TrimSpace(offset) == "" {
			return nil, fmt.Errorf("response has missing pagination offset")
		}
		if offsets[offset] {
			return nil, fmt.Errorf("response has repeated pagination offset %q", offset)
		}
		offsets[offset] = true
		query.Set("offset", offset)
	}
}
