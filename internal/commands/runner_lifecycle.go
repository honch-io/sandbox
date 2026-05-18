package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"honch.dev/honch/internal/config"
	"honch.dev/honch/internal/session"
	"honch.dev/honch/internal/stack"
)

func loadSandboxSession(manager session.Manager) (session.State, bool, error) {
	state, err := manager.Load()
	if err == nil {
		return state, true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return session.State{}, false, nil
	}
	return session.State{}, false, err
}

func sandboxStateHasRunner(state session.State) bool {
	return state.Runner.Adapter != "" || state.Runner.PID > 0 || state.Runner.ControlPath != ""
}

func sandboxStateLooksActive(state session.State) bool {
	return state.Stack.Running || sandboxStateHasRunner(state) || state.Proxy.PID > 0 || state.Proxy.Mode != ""
}

func sandboxHasManagedArtifacts(root string, cfg config.Config) bool {
	if len(sandboxStopProcessIDs(sandboxStopProcessPatterns(root, cfg))) > 0 {
		return true
	}
	if matches, _ := filepath.Glob(filepath.Join(root, cfg.Sandbox.StateDir, "pids", "*.pid")); len(matches) > 0 {
		return true
	}
	if matches, _ := filepath.Glob(filepath.Join(root, cfg.Sandbox.StateDir, "*.control")); len(matches) > 0 {
		return true
	}
	return false
}

func sandboxStopProcessIDs(patterns []string) []int {
	pids := []int{}
	for _, pattern := range patterns {
		pids = append(pids, sandboxProcessIDsFn(pattern)...)
	}
	return pids
}

func stopSandboxAll(ctx context.Context, root string, cfg config.Config, manager session.Manager) error {
	state, exists, err := loadSandboxSession(manager)
	if err != nil {
		return err
	}
	hadArtifacts := sandboxHasManagedArtifacts(root, cfg)
	stackActive := state.Stack.Running || sandboxHasStackArtifacts(root, cfg)
	if !exists && !hadArtifacts {
		return nil
	}
	if exists && !sandboxStateLooksActive(state) && !hadArtifacts {
		return nil
	}
	if err := killSandboxProcessesByPatterns(sandboxStopProcessPatterns(root, cfg)); err != nil {
		return err
	}
	if stackActive {
		if err := stack.New(root).Stop(ctx, cfg); err != nil {
			return err
		}
	}
	if state.Proxy.PID > 0 {
		_ = killProcessFn(state.Proxy.PID)
	}
	if err := removeSandboxControlFiles(root, cfg); err != nil {
		return err
	}
	_ = os.Remove(proxyPIDPath(root, cfg))
	if exists && (sandboxStateLooksActive(state) || hadArtifacts) {
		return manager.Clear()
	}
	return nil
}

func stopSandboxAdapter(ctx context.Context, root string, cfg config.Config, manager session.Manager, adapterName string) error {
	state, exists, err := loadSandboxSession(manager)
	if err != nil {
		return err
	}
	patterns := sandboxAdapterProcessPatterns(root, cfg, adapterName)
	if exists && state.Runner.Adapter != "" && state.Runner.Adapter != adapterName && runnerActive(state.Runner) {
		return fmt.Errorf("sandbox runner %q is active; stop it first with `honch sandbox stop %s`", state.Runner.Adapter, state.Runner.Adapter)
	}
	if !exists && len(sandboxStopProcessIDs(patterns)) == 0 {
		_ = os.Remove(adapterControlPath(root, cfg, adapterName))
		return nil
	}
	if err := killSandboxProcessesByPatterns(patterns); err != nil {
		return err
	}
	if err := os.Remove(adapterControlPath(root, cfg, adapterName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !exists {
		return nil
	}
	if state.Runner.Adapter == adapterName {
		state.Runner = session.RunnerState{}
	}
	return manager.Save(state)
}

func removeSandboxControlFiles(root string, cfg config.Config) error {
	matches, err := filepath.Glob(filepath.Join(root, cfg.Sandbox.StateDir, "*.control"))
	if err != nil {
		return err
	}
	for _, path := range matches {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func sandboxHasStackArtifacts(root string, cfg config.Config) bool {
	matches, err := filepath.Glob(filepath.Join(root, cfg.Sandbox.StateDir, "pids", "*.pid"))
	if err != nil {
		return false
	}
	for _, path := range matches {
		switch filepath.Base(path) {
		case "capture.pid", "worker.pid":
			return true
		}
	}
	return false
}
