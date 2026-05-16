package commands

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"

	"github.com/honch/sdk/tools/sandbox/internal/config"
	"github.com/honch/sdk/tools/sandbox/internal/ui"
	"github.com/spf13/cobra"
)

func newSetupCommand(deps Dependencies) *cobra.Command {
	var yes bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Install or explain missing sandbox prerequisites",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, _, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			actions := setupActions(root, cfg)
			_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.FormatSections("Honch sandbox setup", []ui.Section{
				{Name: "actions", Rows: setupActionRows(actions)},
			}))
			if len(actions) == 0 {
				return nil
			}
			if dryRun {
				printSetupDryRun(cmd.OutOrStdout(), actions)
				return nil
			}
			if !yes {
				ok, err := confirm(cmd.InOrStdin(), cmd.OutOrStdout(), "Run supported setup actions? [y/N] ")
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("setup cancelled")
				}
			}
			return runSetupActions(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), actions)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "run supported setup actions without confirmation")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print setup actions without running them")
	return cmd
}

type setupAction struct {
	Name    string
	Summary string
	Command string
	Run     func(context.Context, io.Writer, io.Writer) error
}

func setupActions(root string, cfg config.Config) []setupAction {
	report := buildSandboxDoctorReport(root, cfg)
	actions := []setupAction{}
	for _, missing := range report.Missing {
		key := fmt.Sprint(missing.Key)
		switch key {
		case "qemu":
			actions = append(actions, setupQEMUAction(root, cfg))
		case "platform", "capture", "worker":
			actions = append(actions, setupManualAction(key, fmt.Sprint(missing.Value)))
		default:
			actions = append(actions, setupHostToolAction(key))
		}
	}
	return compactSetupActions(actions)
}

func setupQEMUAction(root string, cfg config.Config) setupAction {
	plan := newQEMUInstallPlan(managedIDFPath(root, cfg), defaultESPRef)
	return setupAction{
		Name:    "qemu",
		Summary: "install ESP-IDF QEMU tools",
		Command: "honch sandbox qemu install",
		Run: func(ctx context.Context, stdout io.Writer, stderr io.Writer) error {
			return runQEMUInstallPlan(ctx, stdout, stderr, plan)
		},
	}
}

func setupHostToolAction(name string) setupAction {
	if runtime.GOOS == "darwin" && commandStatus("brew") != "missing" {
		if formula := brewPackageForTool(name); formula != "" {
			command := "brew install " + formula
			if name == "docker" {
				command = "brew install --cask docker"
			}
			return setupShellAction(name, "install "+name, command)
		}
	}
	return setupManualAction(name, fmt.Sprint(doctorFix(name).Value))
}

func setupShellAction(name string, summary string, command string) setupAction {
	return setupAction{
		Name:    name,
		Summary: summary,
		Command: command,
		Run: func(ctx context.Context, stdout io.Writer, stderr io.Writer) error {
			return runShellCommand(ctx, stdout, stderr, command)
		},
	}
}

func setupManualAction(name string, summary string) setupAction {
	return setupAction{Name: name, Summary: summary, Command: "manual: " + summary}
}

func brewPackageForTool(name string) string {
	switch name {
	case "python":
		return "python"
	case "bun":
		return "bun"
	case "cargo":
		return "rust"
	case "cmake":
		return "cmake"
	case "docker":
		return "docker"
	default:
		return ""
	}
}

func compactSetupActions(actions []setupAction) []setupAction {
	seen := map[string]bool{}
	result := make([]setupAction, 0, len(actions))
	for _, action := range actions {
		if action.Name == "" || seen[action.Name] {
			continue
		}
		seen[action.Name] = true
		result = append(result, action)
	}
	return result
}

func setupActionRows(actions []setupAction) []ui.Row {
	if len(actions) == 0 {
		return []ui.Row{{Key: "status", Value: "ready"}}
	}
	rows := make([]ui.Row, 0, len(actions))
	for _, action := range actions {
		rows = append(rows, ui.Row{Key: action.Name, Value: action.Summary})
	}
	return rows
}

func printSetupDryRun(out io.Writer, actions []setupAction) {
	_, _ = fmt.Fprintln(out, "dry run")
	for _, action := range actions {
		_, _ = fmt.Fprintf(out, "$ %s\n", action.Command)
	}
}

func runSetupActions(ctx context.Context, stdout io.Writer, stderr io.Writer, actions []setupAction) error {
	for _, action := range actions {
		if action.Run == nil {
			return fmt.Errorf(ui.FormatError("manual setup required", []ui.Row{
				{Key: action.Name, Value: action.Command},
			}))
		}
		if err := action.Run(ctx, stdout, stderr); err != nil {
			return fmt.Errorf("%s setup: %w", action.Name, err)
		}
	}
	return nil
}

func runShellCommand(ctx context.Context, stdout io.Writer, stderr io.Writer, command string) error {
	_, _ = fmt.Fprintf(stdout, "$ %s\n", command)
	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
