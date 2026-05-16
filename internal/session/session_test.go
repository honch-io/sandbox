package session

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestManagerSavesLoadsAndClearsActiveSession(t *testing.T) {
	manager := NewManager(filepath.Join(t.TempDir(), "state.json"))
	started := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	state := State{
		ID:        "sandbox-123",
		StartedAt: started,
		Stack:     StackState{Running: true},
		Runner:    RunnerState{Adapter: "c-core", PID: 42, Detached: true},
		Proxy:     ProxyState{Mode: "online", Port: 18080},
	}

	if err := manager.Save(state); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	loaded, err := manager.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.ID != state.ID || !loaded.StartedAt.Equal(started) || loaded.Runner.Adapter != "c-core" {
		t.Fatalf("loaded state mismatch: %#v", loaded)
	}
	if err := manager.Clear(); err != nil {
		t.Fatalf("Clear returned error: %v", err)
	}
	if _, err := manager.Load(); err == nil {
		t.Fatal("Load succeeded after Clear")
	}
}

func TestManagerSaveWaitsForSessionLock(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(filepath.Join(root, "state.json"))
	lockFile, err := os.OpenFile(filepath.Join(root, "state.json.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer lockFile.Close()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- manager.Save(State{Runner: RunnerState{Adapter: "c-core"}})
	}()

	select {
	case err := <-done:
		t.Fatalf("Save completed while session lock was held: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Save returned error after lock release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Save did not complete after session lock was released")
	}
}
