package github

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// PRState is the state gh reports for one pull request URL, or the reason
// it could not.
type PRState struct {
	State string
	Err   error
}

func (s PRState) String() string {
	if s.Err != nil {
		return "state unknown: " + s.Err.Error()
	}
	return s.State
}

const prStateWorkers = 4

// FetchPRStates asks gh for the state of every URL, a few at a time.
func FetchPRStates(urls []string) map[string]PRState {
	states := make(map[string]PRState, len(urls))
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, prStateWorkers)
	for _, url := range urls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			state := ghPRState(url)
			mu.Lock()
			states[url] = state
			mu.Unlock()
		}()
	}
	wg.Wait()
	return states
}

// ghPRState runs gh pr view for one URL. The first line gh prints on stderr
// becomes the reason when it fails, so an authentication problem is visible.
func ghPRState(url string) PRState {
	ctx, cancel := context.WithTimeout(context.Background(), ghTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", url, "--json", "state")
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		reason, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
		if reason == "" {
			reason = err.Error()
		}
		return PRState{Err: errors.New(reason)}
	}
	var view struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(output, &view); err != nil || view.State == "" {
		return PRState{Err: errors.New("gh printed no state")}
	}
	return PRState{State: view.State}
}
