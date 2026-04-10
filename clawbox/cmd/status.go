package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/surajssd/dotfiles/clawbox/internal/config"
)

var statusCmd = &cobra.Command{
	Use:   "status [session-name]",
	Short: "🔍 Show status of one or all OpenClaw containers",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	if len(args) > 0 && args[0] != "" {
		return runStatusSingle(args[0])
	}
	return runStatusAll()
}

func runStatusSingle(sessionName string) error {
	if _, err := validateAndGetSession(sessionName); err != nil {
		return err
	}

	cname := config.ContainerName(sessionName)

	running, err := appState.Runtime.IsRunning(cname)
	if err != nil {
		return err
	}
	if running {
		fmt.Printf("🟢 Session '%s' is running\n", sessionName)
		// Show container details.
		containers, err := appState.Runtime.ListContainers(cname)
		if err == nil {
			for _, c := range containers {
				if c.Name == cname {
					fmt.Printf("   Name: %s  Status: %s\n", c.Name, c.Status)
				}
			}
		}
		return nil
	}

	exists, err := appState.Runtime.Exists(cname)
	if err != nil {
		return err
	}
	if exists {
		fmt.Printf("🔴 Session '%s' exists but is stopped\n", sessionName)
		return nil
	}

	fmt.Printf("⚪ Session '%s' has no container (not started yet)\n", sessionName)
	return nil
}

func runStatusAll() error {
	fmt.Println("📦 All OpenClaw containers:")
	containers, err := appState.Runtime.ListContainers(config.ContainerPrefix + "-")
	if err != nil {
		return err
	}

	if len(containers) == 0 {
		fmt.Println("  (none)")
		return nil
	}

	for _, c := range containers {
		fmt.Printf("  %s  %s\n", c.Name, c.Status)
	}
	return nil
}
