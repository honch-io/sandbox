package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

const defaultLocalBinName = ".local/bin/honch"

func newInstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "install",
		Short:         "Install honch into your PATH",
		Hidden:        true,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := defaultInstallTarget()
			if err != nil {
				return err
			}
			ok, err := confirm(cmd.InOrStdin(), cmd.OutOrStdout(), fmt.Sprintf("Install honch to %s? [y/N] ", target))
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("install cancelled")
			}

			installed, err := installExecutable(target)
			if err != nil {
				return err
			}
			if installed {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Installed honch to %s\n", target)
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "honch is already installed at %s\n", target)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Reload your shell or run `hash -r` so the new binary is picked up from PATH.")
			return nil
		},
	}
	return cmd
}

func defaultInstallTarget() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, defaultLocalBinName), nil
}

func installExecutable(target string) (bool, error) {
	source, err := os.Executable()
	if err != nil {
		return false, err
	}
	if samePath(source, target) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return false, err
	}
	if err := copyExecutable(source, target); err != nil {
		return false, err
	}
	return true, nil
}

func copyExecutable(source string, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp, err := os.CreateTemp(filepath.Dir(target), ".honch-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	if _, err := io.Copy(tmp, in); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		if err := tmp.Chmod(0o755); err != nil {
			return err
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(tmpPath, target); err != nil {
		return err
	}
	return nil
}

func samePath(a string, b string) bool {
	return resolvedPath(a) == resolvedPath(b)
}

func resolvedPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}
