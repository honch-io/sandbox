package session

import (
	"path/filepath"
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
