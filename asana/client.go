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

type taskInput struct {
	Name     string   `json:"name"`
	Projects []string `json:"projects"`
	Assignee string   `json:"assignee"`
	Notes    string   `json:"notes,omitempty"`
	DueOn    string   `json:"due_on,omitempty"`
}

func createTask(ctx context.Context, deps dependencies, token string, task taskInput) (string, error) {
	body, err := json.Marshal(struct {
		Data taskInput `json:"data"`
	}{Data: task})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deps.apiBaseURL+"/tasks?opt_fields=permalink_url", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("prepare task request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	client := *deps.httpClient
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("create task: %w; the task may exist; check Asana before retrying", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := apiStatusError(resp, "permission denied: check access to the project", "project is missing or inaccessible"); err != nil {
		return "", err
	}
	var result struct {
		Data struct {
			PermalinkURL string `json:"permalink_url"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("read task response: %w; the task may exist; check Asana before retrying", err)
	}
	if strings.TrimSpace(result.Data.PermalinkURL) == "" {
		return "", fmt.Errorf("task response has no permalink_url; the task may exist; check Asana before retrying")
	}
	return result.Data.PermalinkURL, nil
}

func apiStatusError(resp *http.Response, forbidden, notFound string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	message := http.StatusText(resp.StatusCode)
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		message = "authentication failed: check ASANA_ACCESS_TOKEN or token_command"
	case http.StatusForbidden:
		message = forbidden
	case http.StatusNotFound:
		message = notFound
	case http.StatusTooManyRequests:
		message = "rate limit exceeded"
		if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
			message += "; Retry-After: " + retryAfter
		}
	}
	var result struct {
		Errors []struct {
			Message string `json:"message"`
			Phrase  string `json:"phrase"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
		for _, apiError := range result.Errors {
			if apiError.Message != "" {
				message += "; " + apiError.Message
			}
			if apiError.Phrase != "" {
				message += "; " + apiError.Phrase
			}
		}
	}
	return fmt.Errorf("asana HTTP %d: %s", resp.StatusCode, message)
}

type taskProject struct {
	GID  string `json:"gid"`
	Name string `json:"name"`
}

type task struct {
	GID          string        `json:"gid"`
	Name         string        `json:"name"`
	DueOn        *string       `json:"due_on"`
	DueAt        *string       `json:"due_at"`
	PermalinkURL string        `json:"permalink_url"`
	Projects     []taskProject `json:"projects"`
	dueDate      string
}

func fetchWorkspaces(ctx context.Context, deps dependencies, token string) ([]string, error) {
	var result struct {
		Data struct {
			Workspaces []struct {
				GID string `json:"gid"`
			} `json:"workspaces"`
		} `json:"data"`
	}
	if err := getAsana(ctx, deps, token, "/users/me?opt_fields=workspaces", &result); err != nil {
		return nil, err
	}
	if result.Data.Workspaces == nil {
		return nil, fmt.Errorf("user response has no workspaces")
	}
	workspaces := make([]string, 0, len(result.Data.Workspaces))
	for _, workspace := range result.Data.Workspaces {
		if !isGID(workspace.GID) {
			return nil, fmt.Errorf("user response has invalid workspace GID %q", workspace.GID)
		}
		workspaces = append(workspaces, workspace.GID)
	}
	return workspaces, nil
}

func fetchTasks(ctx context.Context, deps dependencies, token, workspace string) ([]task, error) {
	query := url.Values{
		"assignee": {"me"}, "workspace": {workspace}, "completed_since": {"now"}, "limit": {"100"},
		"opt_fields": {"name,due_on,due_at,permalink_url,projects.name"},
	}
	var tasks []task
	offsets := make(map[string]bool)
	for {
		var result struct {
			Data     []task `json:"data"`
			NextPage *struct {
				Offset string `json:"offset"`
			} `json:"next_page"`
		}
		if err := getAsana(ctx, deps, token, "/tasks?"+query.Encode(), &result); err != nil {
			return nil, err
		}
		if result.Data == nil {
			return nil, fmt.Errorf("response has no task data")
		}
		tasks = append(tasks, result.Data...)
		if result.NextPage == nil {
			return tasks, nil
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

func getAsana(ctx context.Context, deps dependencies, token, endpoint string, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, deps.apiBaseURL+endpoint, nil)
	if err != nil {
		return fmt.Errorf("prepare listing request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := *deps.httpClient
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("list Asana tasks: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := apiStatusError(resp, "permission denied: check access to the user and workspace", "user or workspace is missing or inaccessible"); err != nil {
		return err
	}
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("read Asana response: %w", err)
	}
	return nil
}
