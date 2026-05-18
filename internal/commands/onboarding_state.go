package commands

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"honch.dev/honch/internal/config"
)

const onboardingStateVersion = 1

type onboardingState struct {
	Version     int       `json:"version"`
	CompletedAt time.Time `json:"completed_at"`
}

func onboardingStatePath(root string, cfg config.Config) string {
	return filepath.Join(root, cfg.Sandbox.StateDir, "onboarding.json")
}

func onboardingComplete(root string, cfg config.Config) (bool, error) {
	path := onboardingStatePath(root, cfg)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	var state onboardingState
	if err := json.Unmarshal(data, &state); err != nil {
		return false, err
	}
	return state.Version >= onboardingStateVersion, nil
}

func saveOnboardingState(root string, cfg config.Config) error {
	path := onboardingStatePath(root, cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	state := onboardingState{Version: onboardingStateVersion, CompletedAt: time.Now().UTC()}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
