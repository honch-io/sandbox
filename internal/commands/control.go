package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/honch/sdk/tools/sandbox/internal/ui"
	"github.com/spf13/cobra"
)

func liveControlCommand(deps Dependencies, use string, short string, run func(io.Writer) error) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := writeHarnessControl(deps, run); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.Success(fmt.Sprintf("%s control has been sent", cmd.Name())))
			return nil
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
		if errors.Is(err, os.ErrNotExist) {
			return errors.New(ui.FormatError("no active sandbox session", []ui.Row{
				{Key: "start", Value: "honch sandbox start"},
				{Key: "runner", Value: "honch sandbox run <adapter> --detach"},
				{Key: "adapters", Value: "honch sandbox adapters list"},
			}))
		}
		return fmt.Errorf("load sandbox session: %w", err)
	}
	if state.Runner.ControlPath == "" {
		return errors.New(ui.FormatError("no active sandbox runner", []ui.Row{
			{Key: "runner", Value: "honch sandbox run <adapter> --detach"},
			{Key: "adapters", Value: "honch sandbox adapters list"},
			{Key: "status", Value: "honch sandbox status"},
		}))
	}
	f, err := os.OpenFile(state.Runner.ControlPath, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return errors.New(ui.FormatError("harness control is not available", []ui.Row{
			{Key: "control", Value: state.Runner.ControlPath},
			{Key: "status", Value: "honch sandbox status"},
			{Key: "logs", Value: "honch sandbox logs device"},
		}))
	}
	defer f.Close()
	return run(f)
}
