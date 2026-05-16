package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	Repos   ReposConfig   `mapstructure:"repos"`
	Ports   PortsConfig   `mapstructure:"ports"`
	Sandbox SandboxConfig `mapstructure:"sandbox"`
	Stack   StackConfig   `mapstructure:"stack"`
}

type ReposConfig struct {
	Capture  string `mapstructure:"capture"`
	Platform string `mapstructure:"platform"`
	Worker   string `mapstructure:"worker"`
}

type PortsConfig struct {
	Capture    int `mapstructure:"capture"`
	Worker     int `mapstructure:"worker"`
	ClickHouse int `mapstructure:"clickhouse"`
	Proxy      int `mapstructure:"proxy"`
	Control    int `mapstructure:"control"`
}

type SandboxConfig struct {
	ProjectID          string `mapstructure:"project_id"`
	Token              string `mapstructure:"token"`
	ClickHouseDatabase string `mapstructure:"clickhouse_database"`
	EndpointURL        string `mapstructure:"endpoint_url"`
	StateDir           string `mapstructure:"state_dir"`
}

type StackConfig struct {
	StartCommands []CommandConfig `mapstructure:"start_commands"`
	StopCommands  []CommandConfig `mapstructure:"stop_commands"`
}

type CommandConfig struct {
	Repo       string            `mapstructure:"repo"`
	WorkingDir string            `mapstructure:"working_dir"`
	Args       []string          `mapstructure:"args"`
	Env        map[string]string `mapstructure:"env"`
	Background bool              `mapstructure:"background"`
	Log        string            `mapstructure:"log"`
}

func Load(root string) (Config, error) {
	v := viper.New()
	setDefaults(v)

	v.SetConfigFile(filepath.Join(root, "tools", "sandbox", "config", "default.yaml"))
	if err := v.MergeInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) && !os.IsNotExist(err) {
			return Config{}, fmt.Errorf("read default config: %w", err)
		}
	}

	v.SetConfigName(".honch-sandbox")
	v.SetConfigType("yaml")
	v.AddConfigPath(root)
	if err := v.MergeInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if cfg.Sandbox.StateDir == "" {
		cfg.Sandbox.StateDir = filepath.Join(root, ".honch-sandbox")
	}
	return cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("repos.capture", "../capture")
	v.SetDefault("repos.platform", "../platform")
	v.SetDefault("repos.worker", "../worker")
	v.SetDefault("ports.capture", 8001)
	v.SetDefault("ports.worker", 8080)
	v.SetDefault("ports.clickhouse", 8123)
	v.SetDefault("ports.proxy", 18080)
	v.SetDefault("ports.control", 18081)
	v.SetDefault("sandbox.project_id", "00000000-0000-0000-0000-000000000002")
	v.SetDefault("sandbox.token", "honch_e2e_test_key")
	v.SetDefault("sandbox.clickhouse_database", "platform")
	v.SetDefault("sandbox.endpoint_url", "http://127.0.0.1:8001")
	v.SetDefault("sandbox.state_dir", ".honch-sandbox")
	v.SetDefault("stack.start_commands", []map[string]any{
		{"repo": "platform", "working_dir": "infra", "args": []string{"docker", "compose", "up", "-d"}},
		{"repo": "capture", "args": []string{"cargo", "run"}, "background": true, "log": "capture.log"},
		{"repo": "worker", "args": []string{"cargo", "run"}, "background": true, "log": "worker.log"},
	})
	v.SetDefault("stack.stop_commands", []map[string]any{
		{"repo": "platform", "working_dir": "infra", "args": []string{"docker", "compose", "down"}},
	})
}
