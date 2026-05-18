package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"honch.dev/honch/internal/commands"
	"honch.dev/honch/internal/ui"
)

func Main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	root := commands.NewRootCommand(commands.Dependencies{})
	root.SetContext(ctx)
	if err := root.Execute(); err != nil {
		if exitCode := MainExitCode(err); exitCode != 0 {
			if !ui.IsSilentError(err) {
				fmt.Fprintln(os.Stderr, err)
			}
			os.Exit(exitCode)
		}
	}
}

func MainExitCode(err error) int {
	if err == nil || errors.Is(err, context.Canceled) {
		return 0
	}
	return 1
}
