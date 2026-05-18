package commands

import (
	"path/filepath"

	"honch.dev/honch/internal/config"
)

func killSandboxRunnerProcesses(root string, cfg config.Config) error {
	return killSandboxProcessesByPatterns(sandboxRunnerProcessPatterns(root, cfg))
}

func sandboxRunnerProcessPatterns(root string, cfg config.Config) []string {
	buildBinary := filepath.Join(root, cfg.Sandbox.StateDir, "build", "c-core", "honch_sandbox_c_core")
	return []string{
		buildBinary,
		filepath.Join(root, "honch") + " sandbox runner-serve ",
		"idf.py -B " + filepath.Join(root, cfg.Sandbox.StateDir, "build", "esp-idf") + " qemu",
		"qemu-system-xtensa .*" + filepath.Join(root, cfg.Sandbox.StateDir, "build", "esp-idf", "qemu_flash.bin"),
	}
}

func sandboxAdapterProcessPatterns(root string, cfg config.Config, adapter string) []string {
	switch adapter {
	case "c-core":
		return []string{
			filepath.Join(root, cfg.Sandbox.StateDir, "build", "c-core", "honch_sandbox_c_core"),
			filepath.Join(root, "honch") + " sandbox runner-serve c-core ",
		}
	case "esp-idf":
		return append([]string{
			filepath.Join(root, "honch") + " sandbox runner-serve esp-idf ",
		}, sandboxQEMUProcessPatterns(root, cfg)...)
	default:
		return []string{filepath.Join(root, "honch") + " sandbox runner-serve " + adapter + " "}
	}
}

func sandboxQEMUProcessPatterns(root string, cfg config.Config) []string {
	return []string{
		"idf.py -B " + filepath.Join(root, cfg.Sandbox.StateDir, "build", "esp-idf") + " qemu",
		"qemu-system-xtensa .*" + filepath.Join(root, cfg.Sandbox.StateDir, "build", "esp-idf", "qemu_flash.bin"),
	}
}

func sandboxStopProcessPatterns(root string, cfg config.Config) []string {
	return append([]string{
		filepath.Join(root, "honch") + " sandbox proxy-serve",
	}, sandboxRunnerProcessPatterns(root, cfg)...)
}

func killSandboxProcessesByPatterns(patterns []string) error {
	for _, pattern := range patterns {
		for _, pid := range sandboxProcessIDsFn(pattern) {
			_ = killProcessFn(pid)
		}
	}
	return nil
}

func sandboxHasMatchingProcesses(patterns []string) bool {
	for _, pattern := range patterns {
		if len(sandboxProcessIDsFn(pattern)) > 0 {
			return true
		}
	}
	return false
}
