package main

import (
	"fmt"
	"os"
)

func main() {
	if err := newCommand(realDependencies()).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
