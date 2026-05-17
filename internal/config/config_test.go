package config

import (
	"os"
	"path/filepath"
	"testing"
)

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
	if cfg.Ports.Proxy != 19091 {
		t.Fatalf("proxy port override = %d", cfg.Ports.Proxy)
	}
	if cfg.Sandbox.ProjectID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("project id override = %q", cfg.Sandbox.ProjectID)
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
