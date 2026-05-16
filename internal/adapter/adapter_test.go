package adapter

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadRegistryReadsAdapterConfigs(t *testing.T) {
	root := t.TempDir()
	adaptersDir := filepath.Join(root, "tools", "sandbox", "adapters")
	if err := os.MkdirAll(adaptersDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAdapter(t, adaptersDir, "c-core.yaml", `
name: c-core
kind: posix
harness: harnesses/c-core
build:
  tool: cmake
controls:
  transport: newline-json
events:
  source: real-sdk-http-cbor
  sink: real-clickhouse
`)
	writeAdapter(t, adaptersDir, "esp-idf.yaml", `
name: esp-idf
kind: qemu-esp32
harness: harnesses/esp-idf
build:
  tool: idf.py
  target: esp32
run:
  tool: qemu-system-xtensa
`)

	registry, err := LoadRegistry(root)
	if err != nil {
		t.Fatalf("LoadRegistry returned error: %v", err)
	}
	if got, want := registry.Names(), []string{"c-core", "esp-idf"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Names = %#v, want %#v", got, want)
	}
	esp, ok := registry.Get("esp-idf")
	if !ok {
		t.Fatal("esp-idf adapter not found")
	}
	if esp.Kind != "qemu-esp32" || esp.Build.Target != "esp32" || esp.Run.Tool != "qemu-system-xtensa" {
		t.Fatalf("unexpected esp-idf config: %+v", esp)
	}
}

func TestLoadRegistryRejectsDuplicateAdapterNames(t *testing.T) {
	root := t.TempDir()
	adaptersDir := filepath.Join(root, "tools", "sandbox", "adapters")
	if err := os.MkdirAll(adaptersDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAdapter(t, adaptersDir, "one.yaml", "name: c-core\nkind: posix\n")
	writeAdapter(t, adaptersDir, "two.yaml", "name: c-core\nkind: posix\n")

	_, err := LoadRegistry(root)
	if err == nil {
		t.Fatal("LoadRegistry accepted duplicate adapter names")
	}
}

func writeAdapter(t *testing.T, dir string, name string, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
