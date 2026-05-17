package commands_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/honch/sdk/tools/sandbox/internal/config"
)

func TestSandboxConfigListShowsEditableSettings(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootDir, ".honch-sandbox.yaml"), []byte(strings.Join([]string{
		"repos:",
		"  capture: ../custom-capture",
		"ports:",
		"  proxy: 19091",
		"sandbox:",
		"  project_id: 11111111-1111-1111-1111-111111111111",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "config", "list"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("config list returned error: %v\n%s", err, out.String())
	}
	plain := out.String()
	for _, want := range []string{
		"Honch sandbox config",
		"capture (path)",
		"../custom-capture",
		"proxy (int)",
		"19091",
		"project_id (string)",
		"Use 'honch sandbox config set <key> <value>' to update",
		"Use 'honch sandbox config edit' to open the file",
	} {
		if !strings.Contains(strings.ToLower(plain), strings.ToLower(want)) {
			t.Fatalf("config list missing %q:\n%s", want, plain)
		}
	}
}

func TestSandboxConfigInitCreatesStarterFile(t *testing.T) {
	rootDir := t.TempDir()
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "config", "init"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("config init returned error: %v\n%s", err, out.String())
	}

	path := filepath.Join(rootDir, ".honch-sandbox.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("config file not created: %v", err)
	}
	contents := string(data)
	for _, want := range []string{
		"# Honch sandbox overrides.",
		"repos:",
		"ports:",
		"sandbox:",
	} {
		if !strings.Contains(contents, want) {
			t.Fatalf("starter config missing %q:\n%s", want, contents)
		}
	}
}

func TestSandboxConfigSetUpdatesValuesAndKeepsDerivedEndpoint(t *testing.T) {
	rootDir := t.TempDir()
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "config", "set", "ports.capture", "19001"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("config set returned error: %v\n%s", err, out.String())
	}

	cfg, err := config.Load(rootDir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Ports.Capture != 19001 {
		t.Fatalf("capture port = %d, want 19001", cfg.Ports.Capture)
	}
	if cfg.Sandbox.EndpointURL != "http://127.0.0.1:19001" {
		t.Fatalf("endpoint url = %q, want derived capture port", cfg.Sandbox.EndpointURL)
	}
}

func TestSandboxConfigSetRejectsInvalidValues(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "invalid int", args: []string{"sandbox", "config", "set", "ports.capture", "nope"}, want: "invalid integer"},
		{name: "unknown key", args: []string{"sandbox", "config", "set", "nope", "value"}, want: "unsupported config key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := NewRootCommand(Dependencies{RootDir: t.TempDir()})
			root.SetArgs(append([]string{"--plain"}, tc.args...))
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)

			err := root.Execute()
			if err == nil {
				t.Fatal("config set succeeded unexpectedly")
			}
			combined := err.Error() + "\n" + out.String()
			if !strings.Contains(combined, tc.want) {
				t.Fatalf("config set error missing %q:\n%s", tc.want, combined)
			}
		})
	}
}

func TestSandboxConfigEditCreatesFileAndOpensEditor(t *testing.T) {
	rootDir := t.TempDir()
	scriptPath := filepath.Join(rootDir, "fake-editor.sh")
	logPath := filepath.Join(rootDir, "editor.log")
	script := "#!/bin/sh\necho \"$1\" >> \"" + logPath + "\"\nexit 0\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", scriptPath)

	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "config", "edit"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("config edit returned error: %v\n%s", err, out.String())
	}

	if _, err := os.Stat(filepath.Join(rootDir, ".honch-sandbox.yaml")); err != nil {
		t.Fatalf("config edit did not create config file: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("editor log missing: %v", err)
	}
	if !strings.Contains(string(data), ".honch-sandbox.yaml") {
		t.Fatalf("editor did not receive the config file path:\n%s", string(data))
	}
}
