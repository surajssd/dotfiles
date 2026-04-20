package cmd

import (
	"fmt"
	"os"
	"os/exec"
	goruntime "runtime"

	"github.com/spf13/cobra"
	"github.com/surajssd/dotfiles/clawbox/internal/config"
	"github.com/surajssd/dotfiles/clawbox/internal/proxy"
	"github.com/surajssd/dotfiles/clawbox/internal/runtime"
	"github.com/surajssd/dotfiles/clawbox/internal/runtime/apple"
	"github.com/surajssd/dotfiles/clawbox/internal/state"
)

// appState holds shared state initialized during preflight.
var appState struct {
	Sessions     config.SessionsFile
	VMNetGateway string
	Runtime      runtime.Runtime
	ProxyManager *proxy.Manager
}

var rootCmd = &cobra.Command{
	Use:   "clawbox",
	Short: "Manage OpenClaw gateway containers",
	Long:  "🐾 clawbox — Manage OpenClaw gateway containers on macOS",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip preflight for commands that don't need runtime access.
		// This allows help, completion, and bare invocations to work
		// without the container CLI installed or config present.
		switch cmd.Name() {
		case "help", "completion", "bash", "zsh", "fish", "powershell":
			return nil
		}
		// Also skip when the root command is invoked with no subcommand
		// (cobra shows usage/help in this case).
		if cmd.Parent() == nil || cmd == cmd.Root() {
			return nil
		}
		return preflightAndInit()
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(restartCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(recreateCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(execCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(proxyCmd)
}

// Execute runs the root command.
func Execute() {
	// Cobra lazily initializes the help and completion commands,
	// so we override them just before execution.
	rootCmd.InitDefaultHelpCmd()
	rootCmd.InitDefaultCompletionCmd()
	for _, cmd := range rootCmd.Commands() {
		switch cmd.Name() {
		case "help":
			cmd.Short = "❓ Help about any command"
		case "completion":
			cmd.Short = "🏁 Generate the autocompletion script for the specified shell"
		}
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// preflightAndInit performs preflight checks and initializes shared state.
func preflightAndInit() error {
	// Check macOS.
	if goruntime.GOOS != "darwin" {
		return fmt.Errorf("clawbox is only supported on macOS (Darwin)")
	}

	// Check container CLI.
	if _, err := exec.LookPath("container"); err != nil {
		return fmt.Errorf("'container' CLI is not installed. Install it with: brew install container")
	}

	// Initialize runtime.
	rt := apple.New()
	appState.Runtime = rt

	// Check container system is running.
	if err := rt.SystemStatus(); err != nil {
		return fmt.Errorf("container system is not running. Start it with: container system start")
	}

	// Check sessions config exists.
	configPath := config.SessionsConfigPath()
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("sessions config not found at %s", configPath)
	}

	// Load sessions config.
	sessions, err := config.LoadSessions(configPath)
	if err != nil {
		return err
	}
	appState.Sessions = sessions

	// Detect vmnet gateway.
	gateway, err := rt.DetectVMNetGateway()
	if err != nil {
		// Non-fatal — proxy features will be disabled.
		gateway = ""
	}
	appState.VMNetGateway = gateway

	// Initialize proxy manager.
	appState.ProxyManager = proxy.NewManager(gateway, rt)

	return nil
}

// validateAndGetSession validates a session name and returns its config.
func validateAndGetSession(name string) (*config.SessionConfig, error) {
	if err := config.ValidateSessionName(name, appState.Sessions); err != nil {
		return nil, err
	}
	return appState.Sessions.GetSession(name)
}

// printConnectionInfo displays dashboard URL, health URL, and usage hints.
func printConnectionInfo(sessionName string, gatewayPort int) {
	fmt.Println()

	token := state.ReadGatewayToken(sessionName)

	if token != "" {
		fmt.Printf("  🌐 Dashboard: http://localhost:%d/#token=%s\n", gatewayPort, token)
	} else {
		fmt.Printf("  🌐 Dashboard: http://127.0.0.1:%d/\n", gatewayPort)
	}
	fmt.Printf("  💚 Health:    http://127.0.0.1:%d/healthz\n", gatewayPort)
	fmt.Println()
	fmt.Println("📋 To view logs:")
	fmt.Printf("  clawbox logs %s\n", sessionName)
	fmt.Println()
	fmt.Println("📱 To approve a device:")
	fmt.Printf("  clawbox exec %s openclaw devices approve\n", sessionName)
}
