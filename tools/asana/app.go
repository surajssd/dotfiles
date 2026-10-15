package main

import (
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

type dependencies struct {
	stdin      io.Reader
	stdout     io.Writer
	stderr     io.Writer
	now        func() time.Time
	httpClient *http.Client
	apiBaseURL string
}

func realDependencies() dependencies {
	return dependencies{
		stdin:      os.Stdin,
		stdout:     os.Stdout,
		stderr:     os.Stderr,
		now:        time.Now,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		apiBaseURL: "https://app.asana.com/api/1.0",
	}
}

func newCommand(deps dependencies) *cobra.Command {
	var configPath string
	root := &cobra.Command{
		Use:           "asana",
		Short:         "Manage your Asana tasks and project aliases",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetIn(deps.stdin)
	root.SetOut(deps.stdout)
	root.SetErr(deps.stderr)
	root.PersistentFlags().StringVar(&configPath, "config", "", "configuration file (default ~/.asana.yaml)")
	add := &cobra.Command{
		Use: "add", Short: "Create a task or private project", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	add.AddCommand(newAddTaskCommand(deps, &configPath), newAddProjectCommand(deps, &configPath))
	get := &cobra.Command{
		Use: "get", Short: "List your tasks or configured projects", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	get.AddCommand(newGetTaskCommand(deps, &configPath), newGetProjectCommand(deps, &configPath))
	root.AddCommand(add, get)
	return root
}
