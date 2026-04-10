package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var restartCmd = &cobra.Command{
	Use:   "restart <session-name>",
	Short: "🔄 Restart an OpenClaw session container",
	Args:  cobra.ExactArgs(1),
	RunE:  runRestart,
}

func runRestart(cmd *cobra.Command, args []string) error {
	sessionName := args[0]
	fmt.Printf("🔄 Restarting session '%s'\n", sessionName)

	if err := runStop(cmd, args); err != nil {
		return err
	}
	return runStart(cmd, args)
}
