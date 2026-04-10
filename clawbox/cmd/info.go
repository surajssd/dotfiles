package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/surajssd/dotfiles/clawbox/internal/config"
)

var infoCmd = &cobra.Command{
	Use:   "info <session-name>",
	Short: "🔗 Show connection info for a session",
	Args:  cobra.ExactArgs(1),
	RunE:  runInfo,
}

func runInfo(cmd *cobra.Command, args []string) error {
	sessionName := args[0]
	cfg, err := validateAndGetSession(sessionName)
	if err != nil {
		return err
	}

	cname := config.ContainerName(sessionName)
	running, err := appState.Runtime.IsRunning(cname)
	if err != nil {
		return err
	}
	if !running {
		return fmt.Errorf("session '%s' is not running. Start it with: clawbox start %s", sessionName, sessionName)
	}

	printConnectionInfo(sessionName, cfg.Ports.Gateway)
	return nil
}
