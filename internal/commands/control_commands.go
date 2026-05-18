package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"honch.dev/honch/internal/config"
	"honch.dev/honch/internal/proxy"
	"honch.dev/honch/internal/runner"
	"honch.dev/honch/internal/session"
	"honch.dev/honch/internal/ui"
)

func newBatteryCommand(deps Dependencies) *cobra.Command {
	var level int
	cmd := liveControlCommand(deps, "battery", "Set the live harness battery level", func(w io.Writer) error {
		return runner.SendControl(w, "battery", map[string]any{"level": level})
	})
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if level == -1 {
			return errors.New(ui.FormatError("missing battery level", []ui.Row{
				{Key: "required", Value: "honch sandbox battery --level <0-100>"},
				{Key: "example", Value: "honch sandbox battery --level 8"},
			}))
		}
		if level < 0 || level > 100 {
			return errors.New(ui.FormatError("battery level must be between 0 and 100", []ui.Row{
				{Key: "example", Value: "honch sandbox battery --level 8"},
			}))
		}
		return nil
	}
	cmd.Flags().IntVar(&level, "level", -1, "battery level from 0 to 100")
	return cmd
}

func newNetworkCommand(deps Dependencies) *cobra.Command {
	var online, offline, serverError bool
	cmd := &cobra.Command{
		Use:   "network --online|--offline|--server-error",
		Short: "Control the sandbox proxy network mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			count := boolCount(online, offline, serverError)
			if count != 1 {
				return errors.New(ui.FormatError("choose one network mode", []ui.Row{
					{Key: "required", Value: "--online, --offline, or --server-error"},
					{Key: "example", Value: "honch sandbox network --offline"},
				}))
			}
			mode := selectedNetworkMode(offline, serverError)
			root, cfg, manager, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			state, err := manager.Load()
			if err != nil || !state.Stack.Running {
				return errors.New(ui.FormatError("sandbox is not running", []ui.Row{
					{Key: "start", Value: "honch sandbox start"},
					{Key: "example", Value: "honch sandbox network --offline"},
				}))
			}
			if err := writeProxyMode(root, cfg, mode); err != nil {
				return err
			}
			if err := saveProxyStateIfActive(manager, cfg, mode); err != nil {
				return err
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.FormatKeyValues("Network", []ui.Row{{Key: "mode", Value: mode}}))
			return nil
		},
	}
	cmd.Flags().BoolVar(&online, "online", false, "forward SDK HTTP to capture")
	cmd.Flags().BoolVar(&offline, "offline", false, "return network-unavailable errors")
	cmd.Flags().BoolVar(&serverError, "server-error", false, "return HTTP 500 responses")
	return cmd
}

func selectedNetworkMode(offline bool, serverError bool) proxy.Mode {
	if offline {
		return proxy.ModeOffline
	}
	if serverError {
		return proxy.ModeServerError
	}
	return proxy.ModeOnline
}

func saveProxyStateIfActive(manager session.Manager, cfg config.Config, mode proxy.Mode) error {
	state, err := manager.Load()
	if err != nil {
		return nil
	}
	state.Proxy.Mode = mode.String()
	state.Proxy.Port = cfg.Ports.Proxy
	return manager.Save(state)
}

func newTrackCommand(deps Dependencies) *cobra.Command {
	var properties string
	cmd := liveControlCommand(deps, "track <event>", "Ask the harness to emit a custom event", func(w io.Writer) error {
		return runner.SendControl(w, "track", map[string]any{"event": "<event>", "properties": properties})
	})
	cmd.Args = func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return errors.New(ui.FormatError("missing event name", []ui.Row{
				{Key: "required", Value: "honch sandbox track <event>"},
				{Key: "example", Value: "honch sandbox track camera.motion --properties '{\"zone\":\"porch\"}'"},
			}))
		}
		return nil
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		props, err := parseProperties(properties)
		if err != nil {
			return err
		}
		if err := writeHarnessControl(deps, func(w io.Writer) error {
			return runner.SendControl(w, "track", map[string]any{"event": args[0], "properties": props})
		}); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.Success(fmt.Sprintf("track control has been sent: %s", args[0])))
		return nil
	}
	cmd.Flags().StringVar(&properties, "properties", "{}", "JSON object properties")
	return cmd
}

func parseProperties(raw string) (map[string]any, error) {
	props := map[string]any{}
	if raw == "" {
		return props, nil
	}
	if !json.Valid([]byte(raw)) {
		return nil, errors.New(ui.FormatError("properties must be valid JSON", []ui.Row{
			{Key: "example", Value: `--properties '{"zone":"porch"}'`},
		}))
	}
	if err := json.Unmarshal([]byte(raw), &props); err != nil {
		return nil, errors.New(ui.FormatError("properties must be a valid JSON object", []ui.Row{
			{Key: "example", Value: `--properties '{"zone":"porch"}'`},
		}))
	}
	if props == nil {
		return nil, errors.New(ui.FormatError("properties must be a valid JSON object", []ui.Row{
			{Key: "example", Value: `--properties '{"zone":"porch"}'`},
		}))
	}
	return props, nil
}

func newFlushCommand(deps Dependencies) *cobra.Command {
	return liveControlCommand(deps, "flush", "Ask the harness to flush queued events", func(w io.Writer) error {
		return runner.SendControl(w, "flush", nil)
	})
}

func newResetCommand(deps Dependencies) *cobra.Command {
	return liveControlCommand(deps, "reset", "Ask the harness to run SDK reset behavior", func(w io.Writer) error {
		return runner.SendControl(w, "reset", nil)
	})
}
