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
		Short:         "Create an Asana task assigned to yourself",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetIn(deps.stdin)
	root.SetOut(deps.stdout)
	root.SetErr(deps.stderr)
	root.PersistentFlags().StringVar(&configPath, "config", "", "configuration file (default ~/.asana.yaml)")
	root.AddCommand(newAddCommand(deps, &configPath))
	return root
}
