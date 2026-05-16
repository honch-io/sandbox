package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/honch/sdk/tools/sandbox/internal/config"
	"github.com/honch/sdk/tools/sandbox/internal/ui"
	"github.com/spf13/cobra"
)

const defaultESPRef = "v6.0.1"

func newQEMUCommand(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "qemu",
		Short: "Manage ESP-IDF QEMU tooling",
		Args:  rejectUnknownArgs,
		RunE:  commandGroupRunE,
	}
	cmd.AddCommand(newQEMUDoctorCommand(deps), newQEMUInstallCommand(deps))
	return cmd
}

func newQEMUDoctorCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check ESP-IDF QEMU tooling",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, _, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			status := qemuToolStatus(root, cfg)
			_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.FormatSections("ESP-IDF QEMU", status.Sections()))
			if !status.Ready() {
				return qemuNotReadyError()
			}
			return nil
		},
	}
}

func qemuNotReadyError() error {
	return errors.New(ui.FormatError("ESP-IDF QEMU tools are not ready", []ui.Row{
		{Key: "install", Value: "honch sandbox qemu install"},
		{Key: "check", Value: "honch sandbox qemu doctor"},
	}))
}

func newQEMUInstallCommand(deps Dependencies) *cobra.Command {
	var idfPath string
	var ref string
	var yes bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install managed ESP-IDF and QEMU tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, _, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			if idfPath == "" {
				idfPath = managedIDFPath(root, cfg)
			}
			plan := newQEMUInstallPlan(idfPath, ref)
			_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.FormatSections("Install ESP-IDF QEMU", []ui.Section{
				{Rows: []ui.Row{
					{Key: "idf path", Value: idfPath},
					{Key: "ref", Value: ref},
					{Key: "qemu tools", Value: "qemu-xtensa qemu-riscv32"},
				}},
			}))
			if dryRun {
				printQEMUInstallDryRun(cmd.OutOrStdout(), plan)
				return nil
			}
			if !yes {
				ok, err := confirm(cmd.InOrStdin(), cmd.OutOrStdout(), "Download and install ESP-IDF/QEMU tools? [y/N] ")
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("install cancelled")
				}
			}
			return runQEMUInstallPlan(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), plan)
		},
	}
	cmd.Flags().StringVar(&idfPath, "idf-path", "", "ESP-IDF checkout path to create or reuse")
	cmd.Flags().StringVar(&ref, "ref", defaultESPRef, "ESP-IDF git ref to install")
	cmd.Flags().BoolVar(&yes, "yes", false, "run without confirmation")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print install commands without running them")
	return cmd
}

type qemuStatus struct {
	IDFPath    string
	IDFSource  string
	IDFPy      string
	QEMUXtensa string
	Python     string
	Git        string
	Homebrew   string
}

func qemuToolStatus(root string, cfg config.Config) qemuStatus {
	idfPath, source := resolveIDFPath(root, cfg)
	idfPy := commandStatus("idf.py")
	qemuXtensa := commandStatus("qemu-system-xtensa")
	if idfPath != "" {
		if path := filepath.Join(idfPath, "tools", "idf.py"); fileExists(path) {
			idfPy = path
		}
		if qemuXtensa == "missing" {
			qemuXtensa = exportedCommandStatus(idfPath, "qemu-system-xtensa")
		}
	}
	return qemuStatus{
		IDFPath:    valueOr(idfPath, "missing"),
		IDFSource:  valueOr(source, "missing"),
		IDFPy:      idfPy,
		QEMUXtensa: qemuXtensa,
		Python:     pythonStatus(),
		Git:        commandStatus("git"),
		Homebrew:   homebrewStatus(),
	}
}

func (s qemuStatus) Ready() bool {
	return s.IDFPath != "missing" && s.IDFPy != "missing" && s.QEMUXtensa != "missing" && s.Python != "missing"
}

func (s qemuStatus) Sections() []ui.Section {
	return []ui.Section{
		{Name: "toolchain", Rows: []ui.Row{
			{Key: "IDF_PATH", Value: s.IDFPath},
			{Key: "source", Value: s.IDFSource},
		}},
		{Name: "commands", Rows: []ui.Row{
			{Key: "idf.py", Value: s.IDFPy},
			{Key: "qemu-system-xtensa", Value: s.QEMUXtensa},
			{Key: "python", Value: s.Python},
			{Key: "git", Value: s.Git},
			{Key: "brew", Value: s.Homebrew},
		}},
	}
}

func resolveIDFPath(root string, cfg config.Config) (string, string) {
	if path := os.Getenv("IDF_PATH"); path != "" {
		if validIDFPath(path) {
			return path, "env"
		}
	}
	managed := managedIDFPath(root, cfg)
	if validIDFPath(managed) {
		return managed, "managed"
	}
	return "", ""
}

func validIDFPath(path string) bool {
	return fileExists(filepath.Join(path, "export.sh")) && fileExists(filepath.Join(path, "tools", "idf.py"))
}

func managedIDFPath(root string, cfg config.Config) string {
	return filepath.Join(root, cfg.Sandbox.StateDir, "toolchains", "esp-idf")
}

func commandStatus(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return "missing"
	}
	return path
}

func exportedCommandStatus(idfPath string, name string) string {
	exportPath := filepath.Join(idfPath, "export.sh")
	if !fileExists(exportPath) {
		return "missing"
	}
	cmd := exec.Command("bash", "-lc", ". "+qemuShellQuote(exportPath)+" >/dev/null && command -v "+qemuShellQuote(name))
	out, err := cmd.Output()
	if err != nil {
		return "missing"
	}
	return stringTrim(out)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func qemuShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func pythonStatus() string {
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return "missing"
}

func homebrewStatus() string {
	if runtime.GOOS != "darwin" {
		return "not-required"
	}
	return commandStatus("brew")
}

type qemuInstallPlanSpec struct {
	IDFPath string
	Ref     string
	Python  string
}

func newQEMUInstallPlan(idfPath string, ref string) qemuInstallPlanSpec {
	return qemuInstallPlanSpec{IDFPath: idfPath, Ref: ref, Python: pythonExecutable()}
}

func pythonExecutable() string {
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func runQEMUInstallPlan(ctx context.Context, stdout io.Writer, stderr io.Writer, plan qemuInstallPlanSpec) error {
	if plan.IDFPath == "" {
		return errors.New(ui.FormatError("IDF path is required", []ui.Row{
			{Key: "example", Value: "honch sandbox qemu install --idf-path .honch-sandbox/toolchains/esp-idf"},
		}))
	}
	if plan.Python == "" || commandStatus(plan.Python) == "missing" {
		return errors.New(ui.FormatError("python is required to install ESP-IDF tools", []ui.Row{
			{Key: "required", Value: "python3 or python"},
			{Key: "fix", Value: "install Python, then rerun honch sandbox qemu install"},
		}))
	}
	if runtime.GOOS == "darwin" && commandStatus("brew") == "missing" {
		return errors.New(ui.FormatError("Homebrew is required to install ESP-IDF QEMU runtime libraries on macOS", []ui.Row{
			{Key: "required", Value: "brew"},
			{Key: "fix", Value: "install Homebrew, then rerun honch sandbox qemu install"},
		}))
	}
	if info, err := os.Stat(plan.IDFPath); errors.Is(err, os.ErrNotExist) {
		if _, err := exec.LookPath("git"); err != nil {
			return errors.New("git is required to clone ESP-IDF")
		}
		if err := runInstallCommand(ctx, stdout, stderr, "", "git", "clone", "--recursive", "--depth", "1", "--branch", plan.Ref, "https://github.com/espressif/esp-idf.git", plan.IDFPath); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if !info.IsDir() || !validIDFPath(plan.IDFPath) {
		return errors.New(ui.FormatError("existing path is not an ESP-IDF checkout", []ui.Row{
			{Key: "path", Value: plan.IDFPath},
			{Key: "fix", Value: "choose an empty --idf-path or remove the partial checkout"},
		}))
	}
	if runtime.GOOS == "darwin" {
		if err := runInstallCommand(ctx, stdout, stderr, "", "brew", "install", "libgcrypt", "glib", "pixman", "sdl2", "libslirp"); err != nil {
			return err
		}
	}
	if err := runInstallCommand(ctx, stdout, stderr, plan.IDFPath, "./install.sh", "esp32"); err != nil {
		return err
	}
	if err := runInstallCommand(ctx, stdout, stderr, plan.IDFPath, plan.Python, "tools/idf_tools.py", "install", "qemu-xtensa", "qemu-riscv32"); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "\n%s\n", ui.Heading("ESP-IDF QEMU tools installed"))
	_, _ = fmt.Fprintf(stdout, "managed idf path: %s\n", plan.IDFPath)
	_, _ = fmt.Fprintln(stdout, ui.Success("ESP-IDF QEMU tools have been installed"))
	_, _ = fmt.Fprintln(stdout, "next: honch sandbox qemu doctor")
	return nil
}

func printQEMUInstallDryRun(out io.Writer, plan qemuInstallPlanSpec) {
	_, _ = fmt.Fprintln(out, "dry run")
	if !fileExists(plan.IDFPath) {
		_, _ = fmt.Fprintf(out, "$ git clone --recursive --depth 1 --branch %s https://github.com/espressif/esp-idf.git %s\n", plan.Ref, plan.IDFPath)
	}
	if runtime.GOOS == "darwin" {
		_, _ = fmt.Fprintln(out, "$ brew install libgcrypt glib pixman sdl2 libslirp")
	}
	_, _ = fmt.Fprintln(out, "$ ./install.sh esp32")
	_, _ = fmt.Fprintf(out, "$ %s tools/idf_tools.py install qemu-xtensa qemu-riscv32\n", valueOr(plan.Python, "python3"))
}

func runInstallCommand(ctx context.Context, stdout io.Writer, stderr io.Writer, dir string, name string, args ...string) error {
	_, _ = fmt.Fprintf(stdout, "$ %s %s\n", name, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
