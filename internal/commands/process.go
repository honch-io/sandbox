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
		_ = appendProxyLog(root, cfg, fmt.Sprintf("proxy already running on 127.0.0.1:%d\n", cfg.Ports.Proxy))
		return nil, nil
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
	if err := cmd.Process.Release(); err != nil {
		_ = logFile.Close()
		return nil, err
	}
	_ = logFile.Close()
	return cmd.Process, nil
}

func appendProxyLog(root string, cfg config.Config, line string) error {
	logPath := filepath.Join(root, cfg.Sandbox.StateDir, "logs", "proxy.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
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

func startRunnerSupervisor(root string, cfg config.Config, adapter string, target string, controlPath string, env map[string]string) (*os.Process, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	logPath := filepath.Join(root, cfg.Sandbox.StateDir, "logs", "device.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
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
	if err := cmd.Process.Release(); err != nil {
		_ = logFile.Close()
		return nil, err
	}
	_ = logFile.Close()
	return cmd.Process, nil
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

func killPortListener(port int) error {
	if port <= 0 {
		return nil
	}
	// This is a recovery path for stale session state. The happy path uses
	// recorded PIDs, but contributors will often interrupt local runs.
	out, err := exec.Command("lsof", "-tiTCP:"+strconv.Itoa(port), "-sTCP:LISTEN").Output()
	if err != nil {
		return nil
	}
	for _, field := range strings.Fields(string(out)) {
		pid, scanErr := strconv.Atoi(field)
		if scanErr != nil {
			continue
		}
		_ = killProcess(pid)
	}
	return nil
}

func killSandboxRunnerProcesses(root string, cfg config.Config) error {
	buildBinary := filepath.Join(root, cfg.Sandbox.StateDir, "build", "c-core", "honch_sandbox_c_core")
	patterns := []string{
		buildBinary,
		filepath.Join(root, "tools", "sandbox", "honch") + " sandbox runner-serve c-core",
		filepath.Join(root, "tools", "sandbox", "honch") + " sandbox runner-serve esp-idf",
		"idf.py -B " + filepath.Join(root, cfg.Sandbox.StateDir, "build", "esp-idf") + " qemu",
		"qemu-system-xtensa .*" + filepath.Join(root, cfg.Sandbox.StateDir, "build", "esp-idf", "qemu_flash.bin"),
	}
	for _, pattern := range patterns {
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
