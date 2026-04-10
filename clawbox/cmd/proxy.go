package cmd

import (
	"github.com/spf13/cobra"
)

var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "🔌 Manage HTTP proxy for sessions",
}

var proxyStartCmd = &cobra.Command{
	Use:   "start <session-name>",
	Short: "Start the HTTP proxy for a session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		cfg, err := validateAndGetSession(sessionName)
		if err != nil {
			return err
		}
		return appState.ProxyManager.Start(sessionName, cfg)
	},
}

var proxyStopCmd = &cobra.Command{
	Use:   "stop <session-name>",
	Short: "Stop the HTTP proxy for a session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		if _, err := validateAndGetSession(sessionName); err != nil {
			return err
		}
		return appState.ProxyManager.Stop(sessionName)
	},
}

var proxyStatusCmd = &cobra.Command{
	Use:   "status <session-name>",
	Short: "Show HTTP proxy status for a session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		cfg, err := validateAndGetSession(sessionName)
		if err != nil {
			return err
		}
		appState.ProxyManager.Status(sessionName, cfg)
		return nil
	},
}

func init() {
	proxyCmd.AddCommand(proxyStartCmd)
	proxyCmd.AddCommand(proxyStopCmd)
	proxyCmd.AddCommand(proxyStatusCmd)
}
