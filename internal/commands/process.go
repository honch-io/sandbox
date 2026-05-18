package commands

import (
	"os"
	"path/filepath"
	"syscall"

	"honch.dev/honch/internal/config"
)

func ensureControlFIFO(root string, cfg config.Config, adapter string) (string, error) {
	path := adapterControlPath(root, cfg, adapter)
	_ = os.Remove(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func adapterControlPath(root string, cfg config.Config, adapter string) string {
	return filepath.Join(root, cfg.Sandbox.StateDir, adapter+".control")
}
