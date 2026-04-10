package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/surajssd/dotfiles/clawbox/internal/config"
)

var removeCmd = &cobra.Command{
	Use:     "remove <session-name>",
	Aliases: []string{"rm"},
	Short:   "🗑️ Stop and remove an OpenClaw session container",
	Args:    cobra.ExactArgs(1),
	RunE:    runRemove,
}

func runRemove(cmd *cobra.Command, args []string) error {
	sessionName := args[0]
	if _, err := validateAndGetSession(sessionName); err != nil {
		return err
	}

	cname := config.ContainerName(sessionName)

	// Stop proxy first.
	appState.ProxyManager.Stop(sessionName)

	exists, err := appState.Runtime.Exists(cname)
	if err != nil {
		return err
	}
	if exists {
		fmt.Printf("🗑️  Stopping and removing container '%s'\n", cname)
		appState.Runtime.Stop(cname)
		if err := appState.Runtime.Remove(cname); err != nil {
			return fmt.Errorf("removing container: %w", err)
		}
		fmt.Printf("✅ Container '%s' removed\n", cname)
		fmt.Printf("💾 State preserved in %s/%s/\n", config.StateDir(), sessionName)
	} else {
		fmt.Printf("⚠️  Container '%s' does not exist\n", cname)
	}

	return nil
}
