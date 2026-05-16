package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
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
	unlock, err := lockSessionFile(m.path + ".lock")
	if err != nil {
		return err
	}
	defer unlock()
	return writeFileAtomically(m.path, data, 0o600)
}

func (m Manager) Clear() error {
	if err := os.Remove(m.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func lockSessionFile(path string) (func(), error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		closeErr := file.Close()
		if closeErr != nil {
			return nil, errors.Join(err, closeErr)
		}
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}

func writeFileAtomically(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Chmod(perm); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}
