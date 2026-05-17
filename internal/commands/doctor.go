package commands

import (
	"context"
	"fmt"
	"runtime"

	"github.com/honch/sdk/tools/sandbox/internal/config"
	"github.com/honch/sdk/tools/sandbox/internal/stack"
	"github.com/honch/sdk/tools/sandbox/internal/ui"
	"github.com/spf13/cobra"
)

func newDoctorCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check sandbox setup prerequisites",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, _, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			report := buildSandboxDoctorReport(root, cfg)
			_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.FormatSections("Honch sandbox doctor", report.Sections()))
			if !report.Ready() {
				return report.Error()
			}
			return nil
		},
	}
}

type sandboxDoctorReport struct {
	Host    []ui.Row
	Repos   []ui.Row
	Images  []ui.Row
	QEMU    []ui.Row
	Missing []ui.Row
}

func buildSandboxDoctorReport(root string, cfg config.Config) sandboxDoctorReport {
	host := []ui.Row{
		{Key: "git", Value: commandStatus("git")},
		{Key: "python", Value: pythonStatus()},
		{Key: "docker", Value: commandStatus("docker")},
		{Key: "bun", Value: commandStatus("bun")},
		{Key: "cargo", Value: commandStatus("cargo")},
		{Key: "cmake", Value: commandStatus("cmake")},
	}
	if runtime.GOOS == "darwin" {
		host = append(host, ui.Row{Key: "brew", Value: homebrewStatus()})
	}

	repoHealth := stack.New(root).Health(context.Background(), cfg)
	repos := []ui.Row{}
	for _, name := range []string{"platform", "capture", "worker"} {
		if state, ok := repoHealth[name]; ok {
			repos = append(repos, ui.Row{Key: name, Value: state})
		}
	}

	qemuStatus := qemuToolStatus(root, cfg)
	qemu := []ui.Row{
		{Key: "esp-idf", Value: valueOr(qemuStatus.IDFSource, "missing")},
		{Key: "qemu", Value: qemuReadyValue(qemuStatus.Ready())},
	}
	images := dockerImageRows(context.Background(), cfg)

	return sandboxDoctorReport{
		Host:    host,
		Repos:   repos,
		Images:  images,
		QEMU:    qemu,
		Missing: missingDoctorRows(host, repos, images, qemuStatus.Ready()),
	}
}

func qemuReadyValue(ready bool) string {
	if ready {
		return "ready"
	}
	return "run honch sandbox qemu install"
}

func missingDoctorRows(host []ui.Row, repos []ui.Row, images []ui.Row, qemuReady bool) []ui.Row {
	missing := []ui.Row{}
	for _, row := range host {
		if fmt.Sprint(row.Value) == "missing" {
			missing = append(missing, doctorFix(row.Key))
		}
	}
	for _, row := range repos {
		if fmt.Sprint(row.Value) == "missing" {
			missing = append(missing, ui.Row{Key: row.Key, Value: "clone sibling repo beside SDK"})
		}
	}
	for _, row := range images {
		state := fmt.Sprint(row.Value)
		if state == "missing" || state == "docker unhealthy" {
			missing = append(missing, ui.Row{Key: "images", Value: "run honch sandbox images pull"})
			break
		}
	}
	if !qemuReady {
		missing = append(missing, ui.Row{Key: "qemu", Value: "run honch sandbox qemu install"})
	}
	return missing
}

func doctorFix(key string) ui.Row {
	switch key {
	case "python":
		return ui.Row{Key: key, Value: "install Python 3"}
	case "brew":
		return ui.Row{Key: key, Value: "install Homebrew"}
	case "docker":
		return ui.Row{Key: key, Value: "install and start Docker"}
	case "bun":
		return ui.Row{Key: key, Value: "install Bun"}
	case "cargo":
		return ui.Row{Key: key, Value: "install Rust/Cargo"}
	case "cmake":
		return ui.Row{Key: key, Value: "install CMake"}
	case "git":
		return ui.Row{Key: key, Value: "install Git"}
	default:
		return ui.Row{Key: key, Value: "install required tool"}
	}
}

func (r sandboxDoctorReport) Ready() bool {
	return len(r.Missing) == 0
}

func (r sandboxDoctorReport) Sections() []ui.Section {
	sections := []ui.Section{
		{Name: "host", Rows: r.Host},
		{Name: "repos", Rows: r.Repos},
		{Name: "images", Rows: r.Images},
		{Name: "emulator", Rows: r.QEMU},
	}
	if len(r.Missing) > 0 {
		sections = append(sections, ui.Section{Name: "next", Rows: r.Missing})
	}
	return sections
}

func (r sandboxDoctorReport) Error() error {
	if r.Ready() {
		return nil
	}
	return ui.NewSilentError(ui.FormatError("sandbox setup is incomplete", r.Missing))
}
