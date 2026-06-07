package config

import (
	"os"
	"path/filepath"
	"testing"
)

func stringSlicesEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func assertMapEntries(t *testing.T, got map[string]string, want map[string]string) {
	t.Helper()
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("%s = %q, want %q", key, got[key], value)
		}
	}
}

func TestLoadUsesDefaultsAndRootOverride(t *testing.T) {
	root := t.TempDir()
	override := []byte(`
repos:
  capture: ../custom-capture
ports:
  proxy: 19091
sandbox:
  project_id: 11111111-1111-1111-1111-111111111111
`)
	if err := os.WriteFile(filepath.Join(root, ".honch-sandbox.yaml"), override, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Repos.Capture != "../custom-capture" {
		t.Fatalf("capture repo override = %q", cfg.Repos.Capture)
	}
	if cfg.Repos.Platform != "../platform" {
		t.Fatalf("platform repo default = %q", cfg.Repos.Platform)
	}
	if cfg.Repos.Worker != "../platform" {
		t.Fatalf("worker repo default = %q", cfg.Repos.Worker)
	}
	if cfg.Ports.Proxy != 19091 {
		t.Fatalf("proxy port override = %d", cfg.Ports.Proxy)
	}
	if cfg.Sandbox.ProjectID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("project id override = %q", cfg.Sandbox.ProjectID)
	}
}

func TestLoadDefaultsUsePlatformWorkspaceServices(t *testing.T) {
	root := t.TempDir()

	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Repos.Capture != "../platform" {
		t.Fatalf("capture repo default = %q", cfg.Repos.Capture)
	}
	if cfg.Repos.Worker != "../platform" {
		t.Fatalf("worker repo default = %q", cfg.Repos.Worker)
	}
	if cfg.RepoSources.Capture != "" {
		t.Fatalf("capture source default = %q", cfg.RepoSources.Capture)
	}
	if cfg.RepoSources.Worker != "" {
		t.Fatalf("worker source default = %q", cfg.RepoSources.Worker)
	}

	var captureCommand, workerCommand *CommandConfig
	for index := range cfg.Stack.StartCommands {
		command := &cfg.Stack.StartCommands[index]
		switch command.Repo {
		case "capture":
			captureCommand = command
		case "worker":
			workerCommand = command
		}
	}
	if captureCommand == nil {
		t.Fatal("capture start command missing")
	}
	if captureCommand.WorkingDir != "" {
		t.Fatalf("capture working dir = %q", captureCommand.WorkingDir)
	}
	if got, want := captureCommand.Args, []string{"cargo", "run", "-p", "honch-capture"}; !stringSlicesEqual(got, want) {
		t.Fatalf("capture args = %#v, want %#v", got, want)
	}
	assertMapEntries(t, captureCommand.Env, map[string]string{
		"SERVER_ADDR":          "0.0.0.0:8001",
		"PUBSUB_EMULATOR_HOST": "localhost:8085",
		"PUBSUB_PROJECT_ID":    "platform-local",
		"PUBSUB_EVENTS_TOPIC":  "events-raw",
		"REDIS_URL":            "redis://localhost:6379",
		"DATABASE_URL":         "postgresql://platform:platform@localhost:5432/platform",
	})
	if workerCommand == nil {
		t.Fatal("worker start command missing")
	}
	if workerCommand.WorkingDir != "" {
		t.Fatalf("worker working dir = %q", workerCommand.WorkingDir)
	}
	if got, want := workerCommand.Args, []string{"cargo", "run", "-p", "honch-unified-worker"}; !stringSlicesEqual(got, want) {
		t.Fatalf("worker args = %#v, want %#v", got, want)
	}
	assertMapEntries(t, workerCommand.Env, map[string]string{
		"PUBSUB_EMULATOR_HOST":       "localhost:8085",
		"PUBSUB_PROJECT_ID":          "platform-local",
		"EVENTS_PUBSUB_TOPIC":        "events-raw",
		"EVENTS_PUBSUB_SUBSCRIPTION": "events-raw-subscription",
		"CLICKHOUSE_URL":             "http://localhost:8123",
		"CLICKHOUSE_DATABASE":        "platform",
		"DATABASE_URL":               "postgresql://platform:platform@localhost:5432/platform",
		"REDIS_URL":                  "redis://localhost:6379/0",
	})
}

func TestLoadPreservesCommandEnvAsUppercaseFromYAML(t *testing.T) {
	root := t.TempDir()
	defaultDir := filepath.Join(root, "config")
	if err := os.MkdirAll(defaultDir, 0o755); err != nil {
		t.Fatal(err)
	}
	defaultConfig := []byte(`
stack:
  start_commands:
    - repo: capture
      args: [cargo, run, -p, honch-capture]
      env:
        PUBSUB_EMULATOR_HOST: localhost:8085
        REDIS_URL: redis://localhost:6379
      background: true
      log: capture.log
`)
	if err := os.WriteFile(filepath.Join(defaultDir, "default.yaml"), defaultConfig, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(cfg.Stack.StartCommands) != 1 {
		t.Fatalf("start commands = %d, want 1", len(cfg.Stack.StartCommands))
	}
	env := cfg.Stack.StartCommands[0].Env
	assertMapEntries(t, env, map[string]string{
		"PUBSUB_EMULATOR_HOST": "localhost:8085",
		"REDIS_URL":            "redis://localhost:6379",
	})
	if _, ok := env["pubsub_emulator_host"]; ok {
		t.Fatal("env contains lower-case pubsub_emulator_host key")
	}
	if _, ok := env["redis_url"]; ok {
		t.Fatal("env contains lower-case redis_url key")
	}
}

func TestLoadDerivesDefaultEndpointFromCapturePortOverride(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".honch-sandbox.yaml"), []byte("ports:\n  capture: 19001\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Sandbox.EndpointURL != "http://127.0.0.1:19001" {
		t.Fatalf("EndpointURL = %q", cfg.Sandbox.EndpointURL)
	}
}

func TestLoadPreservesExplicitDefaultEndpointOverride(t *testing.T) {
	root := t.TempDir()
	override := []byte(`
ports:
  capture: 19001
sandbox:
  endpoint_url: http://127.0.0.1:8001
`)
	if err := os.WriteFile(filepath.Join(root, ".honch-sandbox.yaml"), override, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Sandbox.EndpointURL != "http://127.0.0.1:8001" {
		t.Fatalf("EndpointURL = %q", cfg.Sandbox.EndpointURL)
	}
}

func TestLoadReadsProxyBindAddress(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".honch-sandbox.yaml"), []byte("sandbox:\n  proxy_bind: 0.0.0.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Sandbox.ProxyBind != "0.0.0.0" {
		t.Fatalf("ProxyBind = %q", cfg.Sandbox.ProxyBind)
	}
}

func TestLoadFallsBackToLoopbackForEmptyProxyBindOverride(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".honch-sandbox.yaml"), []byte("sandbox:\n  proxy_bind: \"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Sandbox.ProxyBind != "127.0.0.1" {
		t.Fatalf("ProxyBind = %q, want loopback fallback", cfg.Sandbox.ProxyBind)
	}
}

func TestLoadRejectsInvalidOverride(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".honch-sandbox.yaml"), []byte("ports:\n  proxy: nope\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(root); err == nil {
		t.Fatal("Load succeeded with invalid override")
	}
}

func TestLoadReadsBundledDefaultConfigBeforeRootOverride(t *testing.T) {
	root := t.TempDir()
	defaultDir := filepath.Join(root, "config")
	if err := os.MkdirAll(defaultDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(defaultDir, "default.yaml"), []byte("ports:\n  proxy: 18181\nsandbox:\n  token: default-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".honch-sandbox.yaml"), []byte("ports:\n  proxy: 19191\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Ports.Proxy != 19191 {
		t.Fatalf("override proxy = %d, want 19191", cfg.Ports.Proxy)
	}
	if cfg.Sandbox.Token != "default-token" {
		t.Fatalf("default token = %q, want default-token", cfg.Sandbox.Token)
	}
}
