package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/surajssd/dotfiles/clawbox/internal/state"
)

var configCmd = &cobra.Command{
	Use:   "config <session-name>",
	Short: "⚙️ Show config file path for a session",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfig,
}

func runConfig(cmd *cobra.Command, args []string) error {
	sessionName := args[0]
	if _, err := validateAndGetSession(sessionName); err != nil {
		return err
	}

	configPath := state.ConfigJSONPath(sessionName)
	if !state.ConfigExists(sessionName) {
		return fmt.Errorf("config file not found: %s. Run 'clawbox setup %s' first", configPath, sessionName)
	}

	fmt.Println(configPath)
	return nil
}
