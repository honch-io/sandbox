package commands

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/session"
)

func TestReleaseDetachedProcessKillsProcessWhenReadinessFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process group signaling is POSIX-only")
	}
	cmd := exec.Command("sh", "-c", "sleep 30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = killProcess(pid)
	})

	_, err := releaseDetachedProcess(cmd, func(int) error {
		return errors.New("not ready")
	})
	if err == nil {
		t.Fatal("releaseDetachedProcess succeeded after readiness failure")
	}
	eventually(t, time.Second, func() bool {
		return !processAlive(pid)
	})
}

func TestProxyReadyCallbackReturnsPIDWriteFailure(t *testing.T) {
	root := t.TempDir()
	cfg := configForTest()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	cfg.Ports.Proxy = listener.Addr().(*net.TCPAddr).Port
	pidDir := filepath.Join(root, cfg.Sandbox.StateDir, "pids")
	if err := os.MkdirAll(filepath.Dir(pidDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidDir, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := waitForProxyReadyAndWritePID(context.Background(), root, cfg, os.Getpid(), time.Second); err == nil {
		t.Fatal("proxy readiness callback ignored PID write failure")
	}
}

func TestSaveForegroundRunnerStateKillsProcessWhenSessionSaveFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process group signaling is POSIX-only")
	}
	cmd := exec.Command("sh", "-c", "sleep 30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = killProcess(pid)
	})

	manager := failingSessionManager(t)
	err := saveForegroundRunnerState(manager, session.State{Runner: session.RunnerState{Adapter: "c-core", PID: pid}}, cmd)
	if err == nil {
		t.Fatal("saveForegroundRunnerState succeeded with an unwritable session path")
	}
	eventually(t, time.Second, func() bool {
		return !processAlive(pid)
	})
	if cmd.ProcessState == nil {
		t.Fatal("foreground process was not waited after save failure")
	}
}

func failingSessionManager(t *testing.T) session.Manager {
	t.Helper()
	root := t.TempDir()
	blocker := filepath.Join(root, "state")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	return session.NewManager(filepath.Join(blocker, "session.json"))
}

func eventually(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
