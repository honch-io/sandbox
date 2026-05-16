package commands

import (
	"errors"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"time"
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
