package cmd

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/surajssd/dotfiles/clawbox/internal/config"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "📝 List all defined sessions and their status",
	Args:    cobra.NoArgs,
	RunE:    runList,
}

func runList(cmd *cobra.Command, args []string) error {
	sessions := appState.Sessions
	names := sessions.SessionNames()

	if len(names) == 0 {
		fmt.Println("No sessions defined.")
		return nil
	}

	// Sort names for consistent output.
	sort.Strings(names)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tPORT\tSTATUS")

	for _, name := range names {
		cfg := sessions[name]
		cname := config.ContainerName(name)

		portStr := "?"
		if cfg.Ports.Gateway > 0 {
			portStr = fmt.Sprintf("%d", cfg.Ports.Gateway)
		}

		statusStr := "NotCreated"
		running, _ := appState.Runtime.IsRunning(cname)
		if running {
			statusStr = "Running"
		} else {
			exists, _ := appState.Runtime.Exists(cname)
			if exists {
				statusStr = "Stopped"
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\n", name, portStr, statusStr)
	}

	w.Flush()
	return nil
}
