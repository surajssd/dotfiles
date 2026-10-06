package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func createProject(ctx context.Context, deps dependencies, token, name, workspace string) (string, error) {
	body, err := json.Marshal(map[string]any{"data": map[string]string{
		"name": name, "workspace": workspace, "privacy_setting": "private",
	}})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deps.apiBaseURL+"/projects?opt_fields=permalink_url,privacy_setting", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("prepare project request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	client := *deps.httpClient
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
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
	if _, err := parseProjectURL(result.Data.PermalinkURL); err != nil {
		return "", fmt.Errorf("project response: %w; the project may exist; check Asana before retrying", err)
	}
	if result.Data.PrivacySetting != "private" {
		return "", fmt.Errorf("project response did not confirm private visibility (privacy_setting %q); check %s before retrying; the alias was not saved", result.Data.PrivacySetting, result.Data.PermalinkURL)
	}
	return result.Data.PermalinkURL, nil
}

type project struct {
	GID          string `json:"gid"`
	Name         string `json:"name"`
	PermalinkURL string `json:"permalink_url"`
	Members      []struct {
		GID string `json:"gid"`
	} `json:"members"`
}

func fetchProjects(ctx context.Context, deps dependencies, token, workspace string) ([]project, error) {
	query := url.Values{
		"workspace": {workspace}, "archived": {"false"}, "limit": {"100"},
		"opt_fields": {"name,permalink_url,members.gid"},
	}
	var projects []project
	offsets := make(map[string]bool)
	for {
		var result struct {
			Data     []project `json:"data"`
			NextPage *struct {
				Offset string `json:"offset"`
			} `json:"next_page"`
		}
		if err := getAsana(ctx, deps, token, "/projects?"+query.Encode(), &result); err != nil {
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
