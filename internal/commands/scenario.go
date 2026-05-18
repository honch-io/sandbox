package commands

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
	"honch.dev/honch/internal/proxy"
	"honch.dev/honch/internal/runner"
	"honch.dev/honch/internal/scenario"
	"honch.dev/honch/internal/ui"
)

func newScenarioCommand(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{Use: "scenario", Short: "Run repeatable sandbox scenarios"}
	cmd.AddCommand(&cobra.Command{
		Use:   "run <file.yaml>",
		Short: "Run a YAML scenario against the live sandbox",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New(ui.FormatError("missing scenario file", []ui.Row{
					{Key: "required", Value: "honch sandbox scenario run <file.yaml>"},
					{Key: "example", Value: "honch sandbox scenario run scenarios/offline.yaml"},
				}))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			sc, err := scenario.Load(args[0])
			if err != nil {
				return err
			}
			if err := runScenario(deps, cmd, sc); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.Success(fmt.Sprintf("scenario has completed: %s (%d steps)", sc.Name, len(sc.Steps))))
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
			timer := time.NewTimer(step.Wait.Duration)
			select {
			case <-cmd.Context().Done():
				timer.Stop()
				return cmd.Context().Err()
			case <-timer.C:
			}
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
	state, err := manager.Load()
	if err != nil || !state.Stack.Running {
		return errors.New(ui.FormatError("sandbox is not running", []ui.Row{
			{Key: "start", Value: "honch sandbox start"},
			{Key: "example", Value: "honch sandbox scenario run <file.yaml>"},
		}))
	}
	if err := writeProxyMode(root, cfg, mode); err != nil {
		return err
	}
	return saveProxyStateIfActive(manager, cfg, mode)
}
