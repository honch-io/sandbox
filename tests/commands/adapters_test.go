package commands_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdaptersListShowsRegisteredAdapters(t *testing.T) {
	rootDir := t.TempDir()
	writeAdapterRegistryForTest(t, rootDir)
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "adapters", "list"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("adapters list returned error: %v", err)
	}
	for _, want := range []string{"Honch adapters", "c-core", "esp-idf", "posix", "qemu-esp32"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("list output missing %q:\n%s", want, out.String())
		}
	}
}

func TestAdaptersShowPrintsAdapterDetails(t *testing.T) {
	rootDir := t.TempDir()
	writeAdapterRegistryForTest(t, rootDir)
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "adapters", "show", "esp-idf"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("adapters show returned error: %v", err)
	}
	for _, want := range []string{"Honch adapter", "esp-idf", "qemu-esp32", "harnesses/esp-idf", "idf.py"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("show output missing %q:\n%s", want, out.String())
		}
	}
}

func TestAdaptersValidateReportsInvalidConfig(t *testing.T) {
	rootDir := t.TempDir()
	dir := filepath.Join(rootDir, "adapters")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("name: broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "adapters", "validate"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("validate accepted invalid config")
	}
	if !strings.Contains(err.Error(), "adapter kind is required") {
		t.Fatalf("validate error did not explain invalid config: %v", err)
	}
}

func TestAdaptersDoctorChecksAdapterRequirements(t *testing.T) {
	rootDir := t.TempDir()
	writeAdapterRegistryForTest(t, rootDir)
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "adapters", "doctor", "c-core"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("adapters doctor returned error: %v", err)
	}
	for _, want := range []string{"Honch adapter doctor", "c-core", "cmake"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out.String())
		}
	}
}
