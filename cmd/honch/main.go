package main

import (
	"fmt"
	"os"

	"github.com/honch/sdk/tools/sandbox/internal/commands"
)

func main() {
	root := commands.NewRootCommand(commands.Dependencies{})
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
