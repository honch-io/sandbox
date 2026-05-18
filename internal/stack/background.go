package stack

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/config"
)

func (s Service) startBackground(ctx context.Context, dir string, cfg config.Config, command config.CommandConfig) error {
	running, err := s.backgroundAlreadyRunning(ctx, cfg, command)
	if err != nil {
		return err
	}
	if running {
		return nil
	}
	cmd := exec.CommandContext(ctx, command.Args[0], command.Args[1:]...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	for key, val := range command.Env {
		cmd.Env = append(cmd.Env, key+"="+val)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var logFile *os.File
	if command.Log != "" {
		logPath := filepath.Join(s.Root, cfg.Sandbox.StateDir, "logs", command.Log)
		if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
			return err
		}
		var err error
		logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return err
		}
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}
	if err := startBackgroundCommand(cmd, func(pid int) error {
		return s.writePID(cfg, command.Repo, pid)
	}); err != nil {
		if logFile != nil {
			_ = logFile.Close()
		}
		return err
	}
	if logFile != nil {
		_ = logFile.Close()
	}
	return nil
}

func startBackgroundCommand(cmd *exec.Cmd, writePID func(int) error) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	pid := cmd.Process.Pid
	if err := writePID(pid); err != nil {
		_ = interruptProcessGroup(pid)
		_ = cmd.Wait()
		return err
	}
	if err := cmd.Process.Release(); err != nil {
		_ = interruptProcessGroup(pid)
		_ = cmd.Wait()
		return err
	}
	return nil
}

func interruptProcessGroup(pid int) error {
	if pid <= 0 {
		return nil
	}
	if err := syscall.Kill(-pid, syscall.SIGINT); err == nil {
		return nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(os.Interrupt)
}

func (s Service) backgroundAlreadyRunning(ctx context.Context, cfg config.Config, command config.CommandConfig) (bool, error) {
	port := 0
	switch command.Repo {
	case "capture":
		port = cfg.Ports.Capture
	case "worker":
		port = cfg.Ports.Worker
	default:
		return false, nil
	}
	dialer := net.Dialer{Timeout: 200 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false, nil
	}
	_ = conn.Close()
	pid, ok := readPIDFile(filepath.Join(s.Root, cfg.Sandbox.StateDir, "pids", command.Repo+".pid"))
	if ok && processAlive(pid) && processCommandMatches(pid, command.Args) {
		return true, nil
	}
	return false, fmt.Errorf("%s port 127.0.0.1:%d is already in use by a non-sandbox process", command.Repo, port)
}

func (s Service) waitForBackgroundPorts(ctx context.Context, cfg config.Config, timeout time.Duration) error {
	for _, check := range []struct {
		name string
		port int
	}{
		{name: "capture", port: cfg.Ports.Capture},
		{name: "worker", port: cfg.Ports.Worker},
	} {
		if check.port <= 0 {
			continue
		}
		deadline := time.Now().Add(timeout)
		for {
			if portOpen(ctx, check.port, 250*time.Millisecond) {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("%s did not become ready on port %d", check.name, check.port)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(250 * time.Millisecond):
			}
		}
	}
	return nil
}

func portOpen(ctx context.Context, port int, timeout time.Duration) bool {
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (s Service) writePID(cfg config.Config, name string, pid int) error {
	if name == "" {
		name = "process"
	}
	dir := filepath.Join(s.Root, cfg.Sandbox.StateDir, "pids")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".pid"), []byte(fmt.Sprintf("%d", pid)), 0o600)
}

func readPIDFile(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func processCommandMatches(pid int, args []string) bool {
	if pid <= 0 || len(args) == 0 {
		return false
	}
	out, err := exec.Command("ps", "-p", fmt.Sprint(pid), "-o", "command=").Output()
	if err != nil {
		return false
	}
	command := strings.TrimSpace(string(out))
	return strings.Contains(command, strings.Join(args, " "))
}

func (s Service) stopBackgroundProcesses(cfg config.Config) error {
	pidDir := filepath.Join(s.Root, cfg.Sandbox.StateDir, "pids")
	entries, err := os.ReadDir(pidDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pid") {
			continue
		}
		path := filepath.Join(pidDir, entry.Name())
		data, readErr := os.ReadFile(path)
		if readErr == nil {
			var pid int
			if _, scanErr := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); scanErr == nil && pid > 0 {
				if killErr := syscall.Kill(-pid, syscall.SIGINT); killErr != nil {
					if process, findErr := os.FindProcess(pid); findErr == nil {
						_ = process.Signal(os.Interrupt)
					}
				}
			}
		}
		if removeErr := os.Remove(path); removeErr != nil {
			return removeErr
		}
	}
	return nil
}
