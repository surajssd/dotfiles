package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

func newAddCommand(deps dependencies, configPath *string) *cobra.Command {
	var project, description, due string
	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Create one task in one project and print its URL",
		Long: `Create a task assigned to the authenticated user. The title must be one
nonblank argument. Projects are aliases from the configuration or Asana
project URLs. Without --project, use default_project from the configuration.

ASANA_ACCESS_TOKEN takes precedence over token_command. Descriptions are
plain text; --description - reads stdin. The due date defaults to today in
local time; --due overrides it.
The command sends one request without retries. Check Asana before retrying
after a transport error, because the task may already exist.`,
		Example: `  asana add "Review the proposal"
  asana add "Fix the build" -p work -d "Investigate the release job."
  asana add "Write the runbook" -d - < notes.md
  asana add "Pay the invoice" --due tomorrow`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(args[0]) == "" {
				return errors.New("title must not be blank")
			}
			cfg, err := readConfig(*configPath)
			if err != nil {
				return err
			}
			if !cmd.Flags().Changed("project") {
				project = cfg.DefaultProject
			}
			gid, err := resolveProject(cfg, project)
			if err != nil {
				return err
			}
			task := taskInput{Name: args[0], Projects: []string{gid}, Assignee: "me", Notes: description}
			task.DueOn, err = parseDue(due, deps.now())
			if err != nil {
				return err
			}
			if description == "-" {
				task.Notes, err = readDescription(cmd.Context(), deps.stdin)
				if err != nil {
					return fmt.Errorf("read description: %w", err)
				}
			}
			token, err := lookupToken(cmd.Context(), cfg.TokenCommand)
			if err != nil {
				return err
			}
			permalink, err := createTask(cmd.Context(), deps, token, task)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintln(deps.stdout, permalink); err != nil {
				return fmt.Errorf("write task URL: %w; task was created at %s", err, permalink)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&project, "project", "p", "", "configured project alias or Asana project URL")
	cmd.Flags().StringVarP(&description, "description", "d", "", "plain-text description; - reads stdin")
	cmd.Flags().StringVar(&due, "due", "today", "due date: YYYY-MM-DD, today, or tomorrow")
	return cmd
}

func readDescription(ctx context.Context, stdin io.Reader) (string, error) {
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(stdin)
		done <- result{string(data), err}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case result := <-done:
		return result.text, result.err
	}
}
