package commands

import (
	"errors"
	"fmt"
	"strings"

	"github.com/honch/sdk/tools/sandbox/internal/adapter"
	"github.com/honch/sdk/tools/sandbox/internal/config"
	"github.com/honch/sdk/tools/sandbox/internal/ui"
	"github.com/spf13/cobra"
)

func newAdaptersCommand(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "adapters",
		Short: "Inspect sandbox adapters",
		Args:  rejectUnknownArgs,
		RunE:  commandGroupRunE,
	}
	cmd.AddCommand(
		newAdaptersListCommand(deps),
		newAdaptersShowCommand(deps),
		newAdaptersValidateCommand(deps),
		newAdaptersDoctorCommand(deps),
	)
	return cmd
}

func newAdaptersListCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered sandbox adapters",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _, _, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			registry, err := adapter.LoadRegistry(root)
			if err != nil {
				return err
			}
			rows := []ui.Row{}
			for _, name := range registry.Names() {
				cfg, _ := registry.Get(name)
				rows = append(rows, ui.Row{Key: cfg.Name, Value: adapterSummary(cfg)})
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.FormatSections("Honch adapters", []ui.Section{{Rows: rows}}))
			return nil
		},
	}
}

func newAdaptersShowCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "show <adapter>",
		Short: "Show sandbox adapter configuration",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New(ui.FormatError("missing adapter name", []ui.Row{
					{Key: "required", Value: "honch sandbox adapters show <adapter>"},
					{Key: "example", Value: "honch sandbox adapters show c-core"},
					{Key: "adapters", Value: "honch sandbox adapters list"},
				}))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadAdapterConfig(deps, args[0])
			if err != nil {
				return err
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.FormatSections("Honch adapter", []ui.Section{
				{Name: "identity", Rows: []ui.Row{
					{Key: "name", Value: cfg.Name},
					{Key: "kind", Value: cfg.Kind},
					{Key: "harness", Value: cfg.Harness},
				}},
				{Name: "build", Rows: []ui.Row{
					{Key: "tool", Value: valueOr(cfg.Build.Tool, "not configured")},
					{Key: "target", Value: valueOr(cfg.Build.Target, "not configured")},
					{Key: "output", Value: valueOr(cfg.Build.Output, "not configured")},
				}},
				{Name: "runtime", Rows: []ui.Row{
					{Key: "run tool", Value: valueOr(cfg.Run.Tool, "not configured")},
					{Key: "emulator", Value: valueOr(cfg.Emulator.Tool, "not configured")},
					{Key: "machine", Value: valueOr(cfg.Emulator.Machine, "not configured")},
					{Key: "network", Value: valueOr(cfg.Emulator.Network, "not configured")},
				}},
				{Name: "controls", Rows: []ui.Row{
					{Key: "transport", Value: valueOr(cfg.Controls.Transport, "not configured")},
					{Key: "path", Value: valueOr(cfg.Controls.Path, "not configured")},
				}},
				{Name: "events", Rows: []ui.Row{
					{Key: "source", Value: valueOr(cfg.Events.Source, "not configured")},
					{Key: "sink", Value: valueOr(cfg.Events.Sink, "not configured")},
				}},
			}))
			return nil
		},
	}
}

func newAdaptersValidateCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate sandbox adapter configs",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _, _, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			registry, err := adapter.LoadRegistry(root)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.FormatSections("Honch adapters", []ui.Section{{Rows: []ui.Row{
				{Key: "configs", Value: fmt.Sprintf("%d valid", len(registry.Names()))},
			}}}))
			return nil
		},
	}
}

func newAdaptersDoctorCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor <adapter>",
		Short: "Check adapter-specific requirements",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New(ui.FormatError("missing adapter name", []ui.Row{
					{Key: "required", Value: "honch sandbox adapters doctor <adapter>"},
					{Key: "example", Value: "honch sandbox adapters doctor c-core"},
					{Key: "adapters", Value: "honch sandbox adapters list"},
				}))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, _, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			adapterConfig, err := loadAdapterConfig(deps, args[0])
			if err != nil {
				return err
			}
			rows := adapterDoctorRows(root, cfg, adapterConfig)
			_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.FormatSections("Honch adapter doctor", []ui.Section{
				{Name: adapterConfig.Name, Rows: rows},
			}))
			if adapterDoctorMissing(rows) {
				return ui.NewSilentError(ui.FormatError("adapter requirements are not ready", rows))
			}
			return nil
		},
	}
}

func loadAdapterConfig(deps Dependencies, name string) (adapter.Config, error) {
	root, _, _, err := loadRuntime(deps)
	if err != nil {
		return adapter.Config{}, err
	}
	registry, err := adapter.LoadRegistry(root)
	if err != nil {
		return adapter.Config{}, err
	}
	cfg, ok := registry.Get(name)
	if !ok {
		return adapter.Config{}, fmt.Errorf("unsupported adapter %q; expected %s", name, registry.SupportedList())
	}
	return cfg, nil
}

func adapterSummary(cfg adapter.Config) string {
	switch cfg.Kind {
	case "posix":
		return "posix native harness"
	case "qemu-esp32":
		return "qemu-esp32 firmware harness"
	default:
		return cfg.Kind
	}
}

func adapterDoctorRows(root string, cfg config.Config, adapterConfig adapter.Config) []ui.Row {
	switch adapterConfig.Kind {
	case "posix":
		return []ui.Row{
			{Key: "adapter", Value: adapterConfig.Name},
			{Key: "cmake", Value: commandStatus("cmake")},
			{Key: "harness", Value: adapterConfig.Harness},
		}
	case "qemu-esp32":
		status := qemuToolStatus(root, cfg)
		return []ui.Row{
			{Key: "adapter", Value: adapterConfig.Name},
			{Key: "idf.py", Value: status.IDFPy},
			{Key: "qemu-system-xtensa", Value: status.QEMUXtensa},
			{Key: "python", Value: status.Python},
			{Key: "install", Value: qemuReadyValue(status.Ready())},
		}
	default:
		return []ui.Row{{Key: "kind", Value: "unsupported: " + adapterConfig.Kind}}
	}
}

func adapterDoctorMissing(rows []ui.Row) bool {
	for _, row := range rows {
		value := fmt.Sprint(row.Value)
		if value == "missing" || strings.HasPrefix(value, "run ") || strings.HasPrefix(value, "unsupported:") {
			return true
		}
	}
	return false
}
