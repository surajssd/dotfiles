package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/surajssd/dotfiles/clawbox/internal/config"
)

var stopCmd = &cobra.Command{
	Use:   "stop <session-name>",
	Short: "🛑 Stop an OpenClaw session container",
	Args:  cobra.ExactArgs(1),
	RunE:  runStop,
}

func runStop(cmd *cobra.Command, args []string) error {
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
		fmt.Printf("🛑 Stopping container '%s'\n", cname)
		appState.Runtime.Stop(cname)
		fmt.Printf("✅ Container '%s' stopped\n", cname)
	} else {
		fmt.Printf("⚠️  Container '%s' is not running\n", cname)
	}

	return nil
}
