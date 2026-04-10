package cmd

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/surajssd/dotfiles/clawbox/internal/config"
	"github.com/surajssd/dotfiles/clawbox/internal/proxy"
)

var execCmd = &cobra.Command{
	Use:                "exec <session-name> [command...]",
	Aliases:            []string{"e"},
	Short:              "🐚 Exec into a running OpenClaw session container",
	Args:               cobra.MinimumNArgs(1),
	DisableFlagParsing: true,
	RunE:               runExec,
}

func runExec(cmd *cobra.Command, args []string) error {
	sessionName := args[0]
	cfg, err := validateAndGetSession(sessionName)
	if err != nil {
		return err
	}

	cname := config.ContainerName(sessionName)

	// Determine command to run.
	execArgs := args[1:]
	if len(execArgs) == 0 {
		execArgs = []string{"bash", "-l"}
	}

	// If proxy is enabled and the command is not "bash", wrap it in bash -lc
	// so that proxy env vars from /etc/profile.d/proxy.sh are loaded.
	if proxy.IsEnabled(cfg) && execArgs[0] != "bash" {
		wrapped := strings.Join(execArgs, " ")
		execArgs = []string{"bash", "-lc", wrapped}
	}

	return appState.Runtime.Exec(cname, true, execArgs)
}
