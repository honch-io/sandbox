package stack

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

func TestStartDoesNotRunCommandsWhenMigrationDeclined(t *testing.T) {
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
	if !errors.Is(err, ErrMigrationDeclined) {
		t.Fatalf("Start error = %v, want ErrMigrationDeclined", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("start command ran after migration decline, stat err: %v", err)
	}
}
