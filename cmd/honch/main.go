package main

import (
	"fmt"
	"os"

	"github.com/honch/sdk/tools/sandbox/internal/commands"
	"github.com/honch/sdk/tools/sandbox/internal/ui"
)

func main() {
	root := commands.NewRootCommand(commands.Dependencies{})
	if err := root.Execute(); err != nil {
		if !ui.IsSilentError(err) {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
