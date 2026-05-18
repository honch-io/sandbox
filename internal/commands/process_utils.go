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
)

var sandboxProcessIDsFn = sandboxProcessIDs
var killProcessFn = killProcess

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

func portIsOpen(ctx context.Context, port int, timeout time.Duration) bool {
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
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

func processCommandLine(pid int) string {
	if pid <= 0 {
		return ""
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func processWorkingDirectory(pid int) string {
	if pid <= 0 {
		return ""
	}
	out, err := exec.Command("lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "n") {
			return strings.TrimPrefix(line, "n")
		}
	}
	return ""
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

func sandboxProcessIDs(pattern string) []int {
	out, err := exec.Command("pgrep", "-f", pattern).Output()
	if err != nil {
		return nil
	}
	pids := []int{}
	for _, field := range strings.Fields(string(out)) {
		pid, scanErr := strconv.Atoi(field)
		if scanErr != nil || pid == os.Getpid() {
			continue
		}
		pids = append(pids, pid)
	}
	return pids
}
