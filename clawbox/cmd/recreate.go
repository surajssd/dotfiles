package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var recreateCmd = &cobra.Command{
	Use:   "recreate <session-name>",
	Short: "♻️ Remove and start a session container from scratch",
	Args:  cobra.ExactArgs(1),
	RunE:  runRecreate,
}

func runRecreate(cmd *cobra.Command, args []string) error {
	sessionName := args[0]

	fmt.Printf("♻️  Recreating session '%s'...\n", sessionName)

	// Remove the existing container (ignore errors if it doesn't exist).
	if err := runRemove(cmd, args); err != nil {
		return fmt.Errorf("remove step failed: %w", err)
	}

	fmt.Println()

	// Start a fresh container.
	if err := runStart(cmd, args); err != nil {
		return fmt.Errorf("start step failed: %w", err)
	}

	return nil
}
