package commands

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/config"
	"github.com/honch/sdk/tools/sandbox/internal/proxy"
)

func ensureControlFIFO(root string, cfg config.Config, adapter string) (string, error) {
	path := filepath.Join(root, cfg.Sandbox.StateDir, adapter+".control")
	_ = os.Remove(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func proxyModePath(root string, cfg config.Config) string {
	return filepath.Join(root, cfg.Sandbox.StateDir, "proxy.mode")
}

func writeProxyMode(root string, cfg config.Config, mode proxy.Mode) error {
	path := proxyModePath(root, cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(mode.String()), 0o600)
}

func startProxyProcess(root string, cfg config.Config) (*os.Process, error) {
	if portIsOpen(context.Background(), cfg.Ports.Proxy, 200*time.Millisecond) {
		if pid, ok := readPID(proxyPIDPath(root, cfg)); ok && processAlive(pid) && processCommandContains(pid, "sandbox proxy-serve") {
			_ = appendProxyLog(root, cfg, fmt.Sprintf("proxy already running on 127.0.0.1:%d\n", cfg.Ports.Proxy))
			return nil, nil
		}
		return nil, fmt.Errorf("proxy port 127.0.0.1:%d is already in use by a non-sandbox process", cfg.Ports.Proxy)
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	logPath := filepath.Join(root, cfg.Sandbox.StateDir, "logs", "proxy.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe, "sandbox", "proxy-serve")
	cmd.Dir = root
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, err
	}
	proc, err := releaseDetachedProcess(cmd, func(pid int) error {
		return waitForProxyReadyAndWritePID(context.Background(), root, cfg, pid, 5*time.Second)
	})
	if err != nil {
		_ = logFile.Close()
		return nil, err
	}
	_ = logFile.Close()
	return proc, nil
}

func waitForProxyReadyAndWritePID(ctx context.Context, root string, cfg config.Config, pid int, timeout time.Duration) error {
	if err := waitForPortReady(ctx, cfg.Ports.Proxy, pid, timeout); err != nil {
		return err
	}
	return writePIDFile(proxyPIDPath(root, cfg), pid)
}

func proxyPIDPath(root string, cfg config.Config) string {
	return filepath.Join(root, cfg.Sandbox.StateDir, "pids", "proxy.pid")
}

func appendProxyLog(root string, cfg config.Config, line string) error {
	logPath := filepath.Join(root, cfg.Sandbox.StateDir, "logs", "proxy.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	_, err = fmt.Fprint(logFile, line)
	return err
}

func portIsOpen(ctx context.Context, port int, timeout time.Duration) bool {
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

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

func releaseDetachedProcess(cmd *exec.Cmd, waitReady func(pid int) error) (*os.Process, error) {
	proc := cmd.Process
	pid := proc.Pid
	if err := waitReady(pid); err != nil {
		_ = killProcess(pid)
		_ = cmd.Wait()
		return nil, err
	}
	if err := proc.Release(); err != nil {
		_ = killProcess(pid)
		_ = cmd.Wait()
		return nil, err
	}
	return proc, nil
}

func killProcess(pid int) error {
	if pid <= 0 {
		return nil
	}
	// Detached sandbox processes run in their own process groups. Signal the
	// group first so supervised children stop with their parent.
	if err := syscall.Kill(-pid, syscall.SIGINT); err == nil {
		return nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(os.Interrupt)
}

func killSandboxRunnerProcesses(root string, cfg config.Config) error {
	for _, pattern := range sandboxRunnerProcessPatterns(root, cfg) {
		out, err := exec.Command("pgrep", "-f", pattern).Output()
		if err != nil {
			continue
		}
		for _, field := range strings.Fields(string(out)) {
			pid, scanErr := strconv.Atoi(field)
			if scanErr != nil || pid == os.Getpid() {
				continue
			}
			_ = killProcess(pid)
		}
	}
	return nil
}

func sandboxRunnerProcessPatterns(root string, cfg config.Config) []string {
	buildBinary := filepath.Join(root, cfg.Sandbox.StateDir, "build", "c-core", "honch_sandbox_c_core")
	return []string{
		buildBinary,
		filepath.Join(root, "tools", "sandbox", "honch") + " sandbox runner-serve ",
		"idf.py -B " + filepath.Join(root, cfg.Sandbox.StateDir, "build", "esp-idf") + " qemu",
		"qemu-system-xtensa .*" + filepath.Join(root, cfg.Sandbox.StateDir, "build", "esp-idf", "qemu_flash.bin"),
	}
}

func readPID(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

func writePIDFile(path string, pid int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o600)
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func processCommandContains(pid int, fragment string) bool {
	if pid <= 0 || fragment == "" {
		return false
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), fragment)
}

func waitForPortReady(ctx context.Context, port int, pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return fmt.Errorf("sandbox process %d exited before port %d became ready", pid, port)
		}
		if portIsOpen(ctx, port, 100*time.Millisecond) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("sandbox process %d did not open port %d within %s", pid, port, timeout)
}

func waitForRunnerReady(ctx context.Context, logPath string, pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return fmt.Errorf("sandbox runner %d exited before reporting ready", pid)
		}
		data, err := os.ReadFile(logPath)
		if err == nil && strings.Contains(string(data), `"ready":true`) {
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

func runnerReadyTimeout() time.Duration {
	if value := os.Getenv("HONCH_SANDBOX_RUNNER_READY_TIMEOUT"); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 30 * time.Second
}
