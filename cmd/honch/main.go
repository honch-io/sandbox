package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/honch/sdk/tools/sandbox/internal/commands"
	"github.com/honch/sdk/tools/sandbox/internal/ui"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	root := commands.NewRootCommand(commands.Dependencies{})
	root.SetContext(ctx)
	if err := root.Execute(); err != nil {
		if exitCode := mainExitCode(err); exitCode != 0 {
			if !ui.IsSilentError(err) {
				fmt.Fprintln(os.Stderr, err)
			}
			os.Exit(exitCode)
		}
	}
}

func mainExitCode(err error) int {
	if err == nil || errors.Is(err, context.Canceled) {
		return 0
	}
	return 1
}
