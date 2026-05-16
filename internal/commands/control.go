package commands

import (
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/spf13/cobra"
)

func liveControlCommand(deps Dependencies, use string, short string, run func(io.Writer) error) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			return writeHarnessControl(deps, run)
		},
	}
}

func writeHarnessControl(deps Dependencies, run func(io.Writer) error) error {
	_, _, manager, err := loadRuntime(deps)
	if err != nil {
		return err
	}
	state, err := manager.Load()
	if err != nil {
		return fmt.Errorf("no active sandbox session: %w", err)
	}
	if state.Runner.ControlPath == "" {
		return fmt.Errorf("active runner has no control path")
	}
	f, err := os.OpenFile(state.Runner.ControlPath, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return fmt.Errorf("open harness control: %w", err)
	}
	defer f.Close()
	return run(f)
}
