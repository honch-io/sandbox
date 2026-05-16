package runner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestStartReturnsBeforeProcessExits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test is POSIX-only")
	}
	script := filepath.Join(t.TempDir(), "sleep.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 2\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	cmd, err := Start(context.Background(), script, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("Start waited for the process to exit")
	}
}
