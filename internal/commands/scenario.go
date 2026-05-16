package commands

import (
	"fmt"
	"io"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/proxy"
	"github.com/honch/sdk/tools/sandbox/internal/runner"
	"github.com/honch/sdk/tools/sandbox/internal/scenario"
	"github.com/spf13/cobra"
)

func newScenarioCommand(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{Use: "scenario", Short: "Run repeatable sandbox scenarios"}
	cmd.AddCommand(&cobra.Command{
		Use:   "run <file.yaml>",
		Short: "Run a YAML scenario against the live sandbox",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sc, err := scenario.Load(args[0])
			if err != nil {
				return err
			}
			if err := runScenario(deps, cmd, sc); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "scenario complete: %s (%d steps)\n", sc.Name, len(sc.Steps))
			return nil
		},
	})
	return cmd
}

func runScenario(deps Dependencies, cmd *cobra.Command, sc scenario.Scenario) error {
	for _, step := range sc.Steps {
		switch {
		case step.Battery != nil:
			if step.Battery.Level < 0 || step.Battery.Level > 100 {
				return fmt.Errorf("battery level must be between 0 and 100")
			}
			if err := sendScenarioControl(deps, "battery", map[string]any{"level": step.Battery.Level}); err != nil {
				return err
			}
		case step.Network != nil:
			if err := applyScenarioNetworkMode(deps, step.Network.Mode); err != nil {
				return err
			}
		case step.Track != nil:
			if step.Track.Event == "" {
				return fmt.Errorf("track step missing event")
			}
			props := map[string]any{}
			if step.Track.Properties != nil {
				props = step.Track.Properties
			}
			if err := sendScenarioControl(deps, "track", map[string]any{"event": step.Track.Event, "properties": props}); err != nil {
				return err
			}
		case step.Wait != nil:
			time.Sleep(step.Wait.Duration)
		case step.Flush != nil:
			if err := sendScenarioControl(deps, "flush", nil); err != nil {
				return err
			}
		case step.Reset != nil:
			if err := sendScenarioControl(deps, "reset", nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func sendScenarioControl(deps Dependencies, action string, fields map[string]any) error {
	return writeHarnessControl(deps, func(w io.Writer) error {
		return runner.SendControl(w, action, fields)
	})
}

func applyScenarioNetworkMode(deps Dependencies, rawMode string) error {
	mode, err := proxy.ParseMode(rawMode)
	if err != nil {
		return err
	}
	root, cfg, manager, err := loadRuntime(deps)
	if err != nil {
		return err
	}
	if err := writeProxyMode(root, cfg, mode); err != nil {
		return err
	}
	return saveProxyStateIfActive(manager, cfg, mode)
}
