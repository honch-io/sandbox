package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/config"
)

func startRunnerSupervisor(root string, cfg config.Config, adapter string, target string, controlPath string, env map[string]string, afterReady func(*os.Process) error) (*os.Process, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	logPath := filepath.Join(root, cfg.Sandbox.StateDir, "logs", "device.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe, "sandbox", "runner-serve", adapter, target, controlPath)
	cmd.Dir = root
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, err
	}
	proc, err := releaseDetachedProcess(cmd, func(pid int) error {
		if err := waitForRunnerReady(context.Background(), logPath, pid, runnerReadyTimeout()); err != nil {
			return err
		}
		if afterReady != nil {
			return afterReady(cmd.Process)
		}
		return nil
	})
	if err != nil {
		_ = logFile.Close()
		return nil, err
	}
	_ = logFile.Close()
	return proc, nil
}

func waitForRunnerReady(ctx context.Context, logPath string, pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return fmt.Errorf("sandbox runner %d exited before reporting ready", pid)
		}
		data, err := os.ReadFile(logPath)
		if err == nil && containsReadyMarker(data) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("sandbox runner %d did not report ready within %s", pid, timeout)
}

func containsReadyMarker(data []byte) bool {
	return len(data) > 0 && strings.Contains(string(data), `"ready":true`)
}

func runnerReadyTimeout() time.Duration {
	if value := os.Getenv("HONCH_SANDBOX_RUNNER_READY_TIMEOUT"); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 30 * time.Second
}
