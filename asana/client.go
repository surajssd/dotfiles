package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	var result struct {
		Data struct {
			PermalinkURL string `json:"permalink_url"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
			Phrase  string `json:"phrase"`
		} `json:"errors"`
	}
	decodeErr := json.NewDecoder(resp.Body).Decode(&result)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := http.StatusText(resp.StatusCode)
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			message = "authentication failed: check ASANA_ACCESS_TOKEN or token_command"
		case http.StatusForbidden:
			message = "permission denied: check access to the project"
		case http.StatusNotFound:
			message = "project is missing or inaccessible"
		case http.StatusTooManyRequests:
			message = "rate limit exceeded"
			if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
				message += "; Retry-After: " + retryAfter
			}
		}
		if decodeErr == nil {
			for _, apiError := range result.Errors {
				if apiError.Message != "" {
					message += "; " + apiError.Message
				}
				if apiError.Phrase != "" {
					message += "; " + apiError.Phrase
				}
			}
		}
		return "", fmt.Errorf("asana HTTP %d: %s", resp.StatusCode, message)
	}
	if decodeErr != nil {
		return "", fmt.Errorf("read task response: %w; the task may exist; check Asana before retrying", decodeErr)
	}
	if strings.TrimSpace(result.Data.PermalinkURL) == "" {
		return "", fmt.Errorf("task response has no permalink_url; the task may exist; check Asana before retrying")
	}
	return result.Data.PermalinkURL, nil
}
