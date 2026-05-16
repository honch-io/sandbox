package adapter

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Name     string         `yaml:"name"`
	Kind     string         `yaml:"kind"`
	Harness  string         `yaml:"harness"`
	Build    BuildConfig    `yaml:"build"`
	Run      RunConfig      `yaml:"run"`
	Emulator EmulatorConfig `yaml:"emulator"`
	Controls ControlConfig  `yaml:"controls"`
	Events   EventConfig    `yaml:"events"`
}

type BuildConfig struct {
	Tool   string `yaml:"tool"`
	Target string `yaml:"target"`
	Output string `yaml:"output"`
}

type RunConfig struct {
	Tool   string `yaml:"tool"`
	Serial string `yaml:"serial"`
}

type EmulatorConfig struct {
	Tool    string `yaml:"tool"`
	Machine string `yaml:"machine"`
	Network string `yaml:"network"`
}

type ControlConfig struct {
	Transport string `yaml:"transport"`
	Path      string `yaml:"path"`
}

type EventConfig struct {
	Source string `yaml:"source"`
	Sink   string `yaml:"sink"`
}

type Registry struct {
	byName map[string]Config
}

func LoadRegistry(root string) (Registry, error) {
	dir := filepath.Join(root, "tools", "sandbox", "adapters")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Registry{}, fmt.Errorf("read adapters: %w", err)
	}
	registry := Registry{byName: map[string]Config{}}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		cfg, err := loadConfig(path)
		if err != nil {
			return Registry{}, err
		}
		if cfg.Name == "" {
			return Registry{}, fmt.Errorf("%s: adapter name is required", path)
		}
		if cfg.Kind == "" {
			return Registry{}, fmt.Errorf("%s: adapter kind is required", path)
		}
		if err := validateConfig(path, cfg); err != nil {
			return Registry{}, err
		}
		if _, exists := registry.byName[cfg.Name]; exists {
			return Registry{}, fmt.Errorf("duplicate adapter %q", cfg.Name)
		}
		registry.byName[cfg.Name] = cfg
	}
	return registry, nil
}

func loadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

func validateConfig(path string, cfg Config) error {
	if cfg.Harness == "" {
		return fmt.Errorf("%s: adapter harness is required", path)
	}
	switch cfg.Kind {
	case "posix":
		if cfg.Build.Tool == "" {
			return fmt.Errorf("%s: posix adapter build.tool is required", path)
		}
		if cfg.Controls.Transport == "" {
			return fmt.Errorf("%s: posix adapter controls.transport is required", path)
		}
	case "qemu-esp32":
		if cfg.Build.Tool == "" {
			return fmt.Errorf("%s: qemu-esp32 adapter build.tool is required", path)
		}
		if cfg.Build.Target == "" {
			return fmt.Errorf("%s: qemu-esp32 adapter build.target is required", path)
		}
		if cfg.Emulator.Tool == "" {
			return fmt.Errorf("%s: qemu-esp32 adapter emulator.tool is required", path)
		}
		if cfg.Controls.Transport == "" {
			return fmt.Errorf("%s: qemu-esp32 adapter controls.transport is required", path)
		}
	default:
		return fmt.Errorf("%s: unsupported adapter kind %q", path, cfg.Kind)
	}
	return nil
}

func (r Registry) Get(name string) (Config, bool) {
	cfg, ok := r.byName[name]
	return cfg, ok
}

func (r Registry) Names() []string {
	names := make([]string, 0, len(r.byName))
	for name := range r.byName {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r Registry) SupportedList() string {
	names := r.Names()
	if len(names) == 0 {
		return "none"
	}
	result := names[0]
	for _, name := range names[1:] {
		result += " or " + name
	}
	return result
}
