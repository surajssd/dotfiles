package main

import (
	"fmt"
	"os"

	"github.com/surajssd/dotfiles/tools/planner/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
