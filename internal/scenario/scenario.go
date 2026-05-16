package scenario

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Scenario struct {
	Name  string `yaml:"name"`
	Steps []Step `yaml:"steps"`
}

type Step struct {
	Battery *BatteryStep `yaml:"battery,omitempty"`
	Network *NetworkStep `yaml:"network,omitempty"`
	Track   *TrackStep   `yaml:"track,omitempty"`
	Wait    *WaitStep    `yaml:"wait,omitempty"`
	Flush   *struct{}    `yaml:"flush,omitempty"`
	Reset   *struct{}    `yaml:"reset,omitempty"`
}

type BatteryStep struct {
	Level int `yaml:"level"`
}

type NetworkStep struct {
	Mode string `yaml:"mode"`
}

type TrackStep struct {
	Event      string         `yaml:"event"`
	Properties map[string]any `yaml:"properties"`
}

type WaitStep struct {
	Duration time.Duration `yaml:"duration"`
}

func Load(path string) (Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Scenario{}, err
	}
	var sc Scenario
	if err := yaml.Unmarshal(data, &sc); err != nil {
		return Scenario{}, fmt.Errorf("parse scenario: %w", err)
	}
	if len(sc.Steps) == 0 {
		return Scenario{}, fmt.Errorf("scenario has no steps")
	}
	for i, step := range sc.Steps {
		if step.actionCount() != 1 {
			return Scenario{}, fmt.Errorf("step %d must define exactly one action", i+1)
		}
	}
	return sc, nil
}

func (w *WaitStep) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		Duration string `yaml:"duration"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(raw.Duration)
	if err != nil {
		return err
	}
	w.Duration = parsed
	return nil
}

func (s Step) actionCount() int {
	count := 0
	if s.Battery != nil {
		count++
	}
	if s.Network != nil {
		count++
	}
	if s.Track != nil {
		count++
	}
	if s.Wait != nil {
		count++
	}
	if s.Flush != nil {
		count++
	}
	if s.Reset != nil {
		count++
	}
	return count
}
