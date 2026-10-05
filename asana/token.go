package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode"
)

func lookupToken(ctx context.Context, command []string) (string, error) {
	if token := os.Getenv("ASANA_ACCESS_TOKEN"); token != "" {
		return token, nil
	}
	if len(command) == 0 {
		return "", errors.New("set ASANA_ACCESS_TOKEN or configure token_command")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.WaitDelay = time.Second
	var stderr strings.Builder
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	detail := strings.TrimSpace(stderr.String())
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return "", fmt.Errorf("token_command failed: %w; stderr: %s", err, detail)
	}
	token := strings.TrimRightFunc(string(output), unicode.IsSpace)
	if token == "" {
		return "", fmt.Errorf("token_command returned an empty token; stderr: %s", detail)
	}
	return token, nil
}
