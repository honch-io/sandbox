package commands

import (
	"io"
	"os"
	"path/filepath"

	"honch.dev/honch/internal/config"
	"honch.dev/honch/internal/session"
	"honch.dev/honch/internal/ui"
)

const installedSandboxRootSuffix = ".local/share/honch/sandbox"

func loadRuntime(deps Dependencies) (string, config.Config, session.Manager, error) {
	root := deps.RootDir
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", config.Config{}, session.Manager{}, err
		}
		root = findRepoRoot(wd)
		if !isSandboxRepoRoot(root) {
			if installedRoot, ok := defaultInstalledSandboxRoot(); ok && isSandboxRepoRoot(installedRoot) {
				root = installedRoot
			}
		}
	}
	cfg, err := config.Load(root)
	if err != nil {
		return "", config.Config{}, session.Manager{}, err
	}
	manager := session.NewManager(filepath.Join(root, cfg.Sandbox.StateDir, "session.json"))
	return root, cfg, manager, nil
}

func defaultInstalledSandboxRoot() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	return filepath.Join(home, filepath.FromSlash(installedSandboxRootSuffix)), true
}

func findRepoRoot(start string) string {
	dir := start
	for {
		if isSandboxRepoRoot(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return start
		}
		dir = parent
	}
}

func isSandboxRepoRoot(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "adapters")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "harnesses")); err != nil {
		return false
	}
	return true
}

func boolCount(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}

func valueOr(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func confirm(in io.Reader, out io.Writer, prompt string) (bool, error) {
	return ui.PromptConfirm(in, out, prompt)
}

func stringTrim(data []byte) string {
	for len(data) > 0 && (data[0] == '\n' || data[0] == '\r' || data[0] == ' ' || data[0] == '\t') {
		data = data[1:]
	}
	for len(data) > 0 {
		last := data[len(data)-1]
		if last != '\n' && last != '\r' && last != ' ' && last != '\t' {
			break
		}
		data = data[:len(data)-1]
	}
	return string(data)
}
