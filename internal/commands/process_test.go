package commands

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

func TestProxySessionPIDFallsBackToPIDFile(t *testing.T) {
	root := t.TempDir()
	cfg := configForTest()
	if err := writePIDFile(proxyPIDPath(root, cfg), 12345); err != nil {
		t.Fatal(err)
	}

	pid := proxySessionPID(root, cfg, session.ProxyState{}, nil)
	if pid != 12345 {
		t.Fatalf("proxySessionPID = %d, want pid file value", pid)
	}
}

func TestResolveProxyPortConflictPromptsAndStopsListeningProcess(t *testing.T) {
	root := t.TempDir()
	cfg := configForTest()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	cfg.Ports.Proxy = listener.Addr().(*net.TCPAddr).Port

	prevLookup := lookupListeningProcess
	prevConfirm := confirmProxyPortReuse
	prevShouldConfirm := shouldConfirmProxyPortReuse
	prevStop := stopProxyProcess
	prevWait := waitForProxyPortClose
	lookupListeningProcess = func(port int) (listeningProcessInfo, bool) {
		return listeningProcessInfo{
			PID:     os.Getpid(),
			Command: "honch sandbox proxy-serve",
			Cwd:     root,
		}, true
	}
	confirmProxyPortReuse = func(stdin io.Reader, out io.Writer, prompt string) (bool, error) {
		if !strings.Contains(prompt, "Stop it and continue starting the sandbox?") {
			t.Fatalf("prompt missing expected warning:\n%s", prompt)
		}
		if !strings.Contains(prompt, "pid:") || !strings.Contains(prompt, "cwd:") {
			t.Fatalf("prompt missing process details:\n%s", prompt)
		}
		return true, nil
	}
	shouldConfirmProxyPortReuse = func(stdin io.Reader, stderr io.Writer) bool { return true }
	var stoppedPID int
	stopProxyProcess = func(pid int) error {
		stoppedPID = pid
		return nil
	}
	waitForProxyPortClose = func(ctx context.Context, port int, timeout time.Duration) error {
		return nil
	}
	t.Cleanup(func() {
		lookupListeningProcess = prevLookup
		confirmProxyPortReuse = prevConfirm
		shouldConfirmProxyPortReuse = prevShouldConfirm
		stopProxyProcess = prevStop
		waitForProxyPortClose = prevWait
	})

	if err := resolveProxyPortConflict(context.Background(), strings.NewReader(""), io.Discard, root, cfg); err != nil {
		t.Fatalf("resolveProxyPortConflict returned error: %v", err)
	}
	if stoppedPID != os.Getpid() {
		t.Fatalf("stopped PID = %d, want %d", stoppedPID, os.Getpid())
	}
}

func TestResolveProxyPortConflictDeclineCancelsStart(t *testing.T) {
	root := t.TempDir()
	cfg := configForTest()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	cfg.Ports.Proxy = listener.Addr().(*net.TCPAddr).Port

	prevLookup := lookupListeningProcess
	prevConfirm := confirmProxyPortReuse
	prevShouldConfirm := shouldConfirmProxyPortReuse
	prevStop := stopProxyProcess
	prevWait := waitForProxyPortClose
	lookupListeningProcess = func(port int) (listeningProcessInfo, bool) {
		return listeningProcessInfo{PID: os.Getpid(), Command: "honch sandbox proxy-serve", Cwd: root}, true
	}
	confirmProxyPortReuse = func(stdin io.Reader, out io.Writer, prompt string) (bool, error) {
		return false, nil
	}
	shouldConfirmProxyPortReuse = func(stdin io.Reader, stderr io.Writer) bool { return true }
	var stopped bool
	stopProxyProcess = func(pid int) error {
		stopped = true
		return nil
	}
	waitForProxyPortClose = func(ctx context.Context, port int, timeout time.Duration) error {
		return nil
	}
	t.Cleanup(func() {
		lookupListeningProcess = prevLookup
		confirmProxyPortReuse = prevConfirm
		shouldConfirmProxyPortReuse = prevShouldConfirm
		stopProxyProcess = prevStop
		waitForProxyPortClose = prevWait
	})

	err = resolveProxyPortConflict(context.Background(), strings.NewReader(""), io.Discard, root, cfg)
	if err == nil {
		t.Fatal("resolveProxyPortConflict succeeded after decline")
	}
	if !strings.Contains(err.Error(), "start cancelled") {
		t.Fatalf("decline error did not report cancellation: %v", err)
	}
	if stopped {
		t.Fatal("decline path stopped a process")
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

func TestClearForegroundRunnerStateReturnsSaveFailure(t *testing.T) {
	root := t.TempDir()
	sessionPath := filepath.Join(root, "session.json")
	manager := session.NewManager(sessionPath)
	if err := manager.Save(session.State{Runner: session.RunnerState{Adapter: "c-core", PID: 123}}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sessionPath, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(root, 0o700)
		_ = os.Chmod(sessionPath, 0o600)
	})

	if err := clearForegroundRunnerState(manager); err == nil {
		t.Fatal("clearForegroundRunnerState ignored session save failure")
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
