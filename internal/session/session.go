package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type State struct {
	ID        string      `json:"id"`
	StartedAt time.Time   `json:"started_at"`
	Stack     StackState  `json:"stack"`
	Runner    RunnerState `json:"runner"`
	Proxy     ProxyState  `json:"proxy"`
}

type StackState struct {
	Running bool `json:"running"`
}

type RunnerState struct {
	Adapter     string `json:"adapter,omitempty"`
	PID         int    `json:"pid,omitempty"`
	Detached    bool   `json:"detached,omitempty"`
	ControlPath string `json:"control_path,omitempty"`
}

type ProxyState struct {
	Mode string `json:"mode"`
	Port int    `json:"port"`
	PID  int    `json:"pid,omitempty"`
}

type Manager struct {
	path string
}

func NewManager(path string) Manager {
	return Manager{path: path}
}

func (m Manager) Load() (State, error) {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("decode session state: %w", err)
	}
	return state, nil
}

func (m Manager) Save(state State) error {
	if state.ID == "" {
		state.ID = fmt.Sprintf("sandbox-%d", time.Now().Unix())
	}
	if state.StartedAt.IsZero() {
		state.StartedAt = time.Now().UTC()
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, data, 0o600)
}

func (m Manager) Clear() error {
	if err := os.Remove(m.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
