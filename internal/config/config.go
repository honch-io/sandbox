package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Repos       ReposConfig       `mapstructure:"repos"`
	RepoSources RepoSourcesConfig `mapstructure:"repo_sources"`
	Ports       PortsConfig       `mapstructure:"ports"`
	Sandbox     SandboxConfig     `mapstructure:"sandbox"`
	Stack       StackConfig       `mapstructure:"stack"`
}

type ReposConfig struct {
	Capture  string `mapstructure:"capture"`
	Platform string `mapstructure:"platform"`
	Worker   string `mapstructure:"worker"`
}

type RepoSourcesConfig struct {
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
	ProxyBind          string `mapstructure:"proxy_bind"`
	StateDir           string `mapstructure:"state_dir"`
	IDFPath            string `mapstructure:"idf_path"`
}

type StackConfig struct {
	Images        []string        `mapstructure:"images"`
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

	v.SetConfigFile(filepath.Join(root, "config", "default.yaml"))
	if err := v.MergeInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) && !os.IsNotExist(err) {
			return Config{}, fmt.Errorf("read default config: %w", err)
		}
	}

	v.SetConfigName(".honch-sandbox")
	v.SetConfigType("yaml")
	v.AddConfigPath(root)
	rootConfigPath := ""
	if err := v.MergeInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
	} else {
		rootConfigPath = v.ConfigFileUsed()
	}
	explicitEndpointURL, err := configFileSetsEndpointURL(rootConfigPath)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if cfg.Sandbox.StateDir == "" {
		cfg.Sandbox.StateDir = filepath.Join(root, ".honch-sandbox")
	}
	cfg.Sandbox.EndpointURL = resolvedEndpointURL(cfg, explicitEndpointURL)
	return cfg, nil
}

func resolvedEndpointURL(cfg Config, explicitEndpointURL bool) string {
	const defaultCapturePort = 8001
	const defaultEndpointURL = "http://127.0.0.1:8001"
	if cfg.Sandbox.EndpointURL == "" || (!explicitEndpointURL && cfg.Sandbox.EndpointURL == defaultEndpointURL && cfg.Ports.Capture != defaultCapturePort) {
		return fmt.Sprintf("http://127.0.0.1:%d", cfg.Ports.Capture)
	}
	return cfg.Sandbox.EndpointURL
}

func configFileSetsEndpointURL(path string) (bool, error) {
	if path == "" {
		return false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	var raw struct {
		Sandbox struct {
			EndpointURL *string `yaml:"endpoint_url"`
		} `yaml:"sandbox"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return false, err
	}
	return raw.Sandbox.EndpointURL != nil && strings.TrimSpace(*raw.Sandbox.EndpointURL) != "", nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("repos.capture", "../capture")
	v.SetDefault("repos.platform", "../platform")
	v.SetDefault("repos.worker", "../worker")
	v.SetDefault("repo_sources.capture", "https://github.com/honch-io/capture.git")
	v.SetDefault("repo_sources.platform", "https://github.com/honch-io/platform.git")
	v.SetDefault("repo_sources.worker", "https://github.com/honch-io/worker.git")
	v.SetDefault("ports.capture", 8001)
	v.SetDefault("ports.worker", 8080)
	v.SetDefault("ports.clickhouse", 8123)
	v.SetDefault("ports.proxy", 18080)
	v.SetDefault("ports.control", 18081)
	v.SetDefault("sandbox.project_id", "00000000-0000-0000-0000-000000000002")
	v.SetDefault("sandbox.token", "honch_e2e_test_key")
	v.SetDefault("sandbox.clickhouse_database", "platform")
	v.SetDefault("sandbox.endpoint_url", "http://127.0.0.1:8001")
	v.SetDefault("sandbox.proxy_bind", "127.0.0.1")
	v.SetDefault("sandbox.state_dir", ".honch-sandbox")
	v.SetDefault("stack.images", []string{
		"postgres:16-alpine",
		"redis:7-alpine",
		"clickhouse/clickhouse-server:24.8",
		"gcr.io/google.com/cloudsdktool/cloud-sdk:emulators",
	})
	v.SetDefault("stack.start_commands", []map[string]any{
		{"repo": "platform", "working_dir": "infra", "args": []string{"docker", "compose", "up", "-d"}},
		{"repo": "capture", "args": []string{"cargo", "run"}, "background": true, "log": "capture.log"},
		{"repo": "worker", "args": []string{"cargo", "run"}, "background": true, "log": "worker.log"},
	})
	v.SetDefault("stack.stop_commands", []map[string]any{
		{"repo": "platform", "working_dir": "infra", "args": []string{"docker", "compose", "down"}},
	})
}
