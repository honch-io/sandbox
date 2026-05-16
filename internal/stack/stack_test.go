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
	"testing"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/config"
)

func TestStartRunsBackgroundCommandsFromConfiguredSubdirectory(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "service")
	workdir := filepath.Join(repo, "infra")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "ran.txt")

	cfg := config.Config{
		Repos:   config.ReposConfig{Capture: "service"},
		Sandbox: config.SandboxConfig{StateDir: ".state"},
		Stack: config.StackConfig{StartCommands: []config.CommandConfig{
			{
				Repo:       "capture",
				WorkingDir: "infra",
				Args:       []string{"sh", "-c", "pwd > " + output},
				Background: true,
				Log:        "capture.log",
			},
		}},
	}
	service := New(root)

	if err := service.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	var data []byte
	var err error
	for i := 0; i < 20; i++ {
		data, err = os.ReadFile(output)
		if err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("background command did not write output: %v", err)
	}
	want, err := filepath.EvalSymlinks(workdir)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != want+"\n" {
		t.Fatalf("background command ran in %q, want %q", got, want+"\n")
	}
	if _, err := os.Stat(filepath.Join(root, ".state", "pids", "capture.pid")); err != nil {
		t.Fatalf("background pid file missing: %v", err)
	}
}

func TestStartBackgroundCommandReleasesProcessAfterPIDWrite(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 0")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var wrotePID int

	if err := startBackgroundCommand(cmd, func(pid int) error {
		wrotePID = pid
		return nil
	}); err != nil {
		t.Fatalf("startBackgroundCommand returned error: %v", err)
	}
	if wrotePID <= 0 {
		t.Fatalf("pid was not written: %d", wrotePID)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("released background process remained waitable")
	}
}

func TestStopRemovesBackgroundPidFiles(t *testing.T) {
	root := t.TempDir()
	pidDir := filepath.Join(root, ".state", "pids")
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pidDir, "capture.pid"), []byte("999999"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Sandbox: config.SandboxConfig{StateDir: ".state"}}

	if err := New(root).Stop(context.Background(), cfg); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(pidDir, "capture.pid")); !os.IsNotExist(err) {
		t.Fatalf("pid file still exists or unexpected error: %v", err)
	}
}

func TestStartSkipsMigrationsWhenMigrationDeclined(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "platform")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "ran.txt")
	cfg := config.Config{
		Repos:   config.ReposConfig{Platform: "platform"},
		Sandbox: config.SandboxConfig{StateDir: ".state"},
		Stack: config.StackConfig{StartCommands: []config.CommandConfig{
			{
				Repo: "platform",
				Args: []string{"sh", "-c", "touch " + output},
			},
		}},
	}
	service := New(root)
	service.ApproveMigrations = func() (bool, error) {
		return false, nil
	}

	err := service.Start(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("start command did not run after migration decline: %v", err)
	}
}

func TestStartRejectsUnownedServicePort(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "capture")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	cfg := config.Config{
		Repos:   config.ReposConfig{Capture: "capture"},
		Ports:   config.PortsConfig{Capture: port},
		Sandbox: config.SandboxConfig{StateDir: ".state"},
		Stack: config.StackConfig{StartCommands: []config.CommandConfig{
			{
				Repo:       "capture",
				Args:       []string{"sh", "-c", "sleep 1"},
				Background: true,
				Log:        "capture.log",
			},
		}},
	}

	err = New(root).Start(context.Background(), cfg)
	if err == nil {
		t.Fatal("Start accepted an occupied capture port without sandbox ownership")
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("error did not explain occupied port ownership: %v", err)
	}
}

func TestStartRejectsOccupiedServicePortWithStaleLivePID(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "capture")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	pidDir := filepath.Join(root, ".state", "pids")
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pidDir, "capture.pid"), []byte(fmt.Sprintf("%d", os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Repos:   config.ReposConfig{Capture: "capture"},
		Ports:   config.PortsConfig{Capture: port},
		Sandbox: config.SandboxConfig{StateDir: ".state"},
		Stack: config.StackConfig{StartCommands: []config.CommandConfig{
			{
				Repo:       "capture",
				Args:       []string{"sh", "-c", "sleep 1"},
				Background: true,
				Log:        "capture.log",
			},
		}},
	}

	err = New(root).Start(context.Background(), cfg)
	if err == nil {
		t.Fatal("Start accepted an occupied capture port with unrelated live PID")
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("error did not explain occupied port ownership: %v", err)
	}
}
