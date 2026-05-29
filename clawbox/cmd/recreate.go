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

	// Look up the session config so we know which image to verify. We do this
	// before remove so a typo in the session name fails fast.
	cfg, err := validateAndGetSession(sessionName)
	if err != nil {
		return err
	}

	// Remove the existing container (ignore errors if it doesn't exist).
	if err := runRemove(cmd, args); err != nil {
		return fmt.Errorf("remove step failed: %w", err)
	}

	fmt.Println()

	// Compare the local image digest against the upstream registry and pull
	// if they differ, so the freshly-started container picks up any newer
	// image published under the same tag. Failures here are non-fatal — the
	// helper logs a warning and we proceed with whatever is on disk.
	ensureLatestImage(appState.Runtime, cfg.Image)

	fmt.Println()

	// Start a fresh container.
	if err := runStart(cmd, args); err != nil {
		return fmt.Errorf("start step failed: %w", err)
	}

	return nil
}
