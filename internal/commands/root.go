package commands

import (
	"fmt"
	"io"
	"os"

	"github.com/honch/sdk/tools/sandbox/internal/ui"
	"github.com/spf13/cobra"
)

type Dependencies struct {
	RootDir string
	In      io.Reader
	Out     io.Writer
	Err     io.Writer
}

func NewRootCommand(deps Dependencies) *cobra.Command {
	deps = withDefaultIO(deps)
	var plain bool
	root := &cobra.Command{
		Use:           "honch",
		Short:         "Honch developer tooling",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ui.SetPlain(plain)
		},
	}
	root.PersistentFlags().BoolVar(&plain, "plain", false, "disable styled output")
	root.SetIn(deps.In)
	root.SetOut(deps.Out)
	root.SetErr(deps.Err)
	root.AddCommand(newSandboxCommand(deps))
	installHelp(root)
	return root
}

func withDefaultIO(deps Dependencies) Dependencies {
	if deps.In == nil {
		deps.In = os.Stdin
	}
	if deps.Out == nil {
		deps.Out = os.Stdout
	}
	if deps.Err == nil {
		deps.Err = os.Stderr
	}
	return deps
}

func newSandboxCommand(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sandbox",
		Short: "Run the Honch SDK E2E sandbox",
		Args:  rejectUnknownArgs,
		RunE:  commandGroupRunE,
	}
	cmd.AddCommand(
		newDoctorCommand(deps),
		newSetupCommand(deps),
		newImagesCommand(deps),
		newStartCommand(deps),
		newStopCommand(deps),
		newStatusCommand(deps),
		newUpdateCommand(deps),
		newRunCommand(deps),
		newAdaptersCommand(deps),
		newBatteryCommand(deps),
		newNetworkCommand(deps),
		newTrackCommand(deps),
		newFlushCommand(deps),
		newResetCommand(deps),
		newLogsCommand(deps),
		newEventsCommand(deps),
		newScenarioCommand(deps),
		newQEMUCommand(deps),
		newProxyServeCommand(deps),
		newRunnerServeCommand(deps),
	)
	return cmd
}

func rejectUnknownArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return fmt.Errorf("unknown command %q for %s", args[0], cmd.CommandPath())
}

func commandGroupRunE(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return rejectUnknownArgs(cmd, args)
	}
	return cmd.Help()
}
