package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, realDependencies(), os.Args[1:])
	stop()
	os.Exit(code)
}

func run(ctx context.Context, deps dependencies, args []string) int {
	if args == nil {
		args = []string{}
	}
	cmd := newCommand(deps)
	cmd.SetArgs(args)
	if err := cmd.ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintln(deps.stderr, "error:", err)
		return 1
	}
	return 0
}
