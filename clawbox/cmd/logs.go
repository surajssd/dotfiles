package cmd

import (
	"github.com/spf13/cobra"
	"github.com/surajssd/dotfiles/clawbox/internal/config"
)

var logsCmd = &cobra.Command{
	Use:   "logs <session-name>",
	Short: "📋 Follow logs of an OpenClaw session container",
	Args:  cobra.ExactArgs(1),
	RunE:  runLogs,
}

func runLogs(cmd *cobra.Command, args []string) error {
	sessionName := args[0]
	if _, err := validateAndGetSession(sessionName); err != nil {
		return err
	}

	cname := config.ContainerName(sessionName)
	return appState.Runtime.LogsFollow(cname)
}
