package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/surajssd/dotfiles/tools/asana/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, os.Args[1:])
	stop()
	os.Exit(code)
}
