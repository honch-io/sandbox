package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintLogsHonorsTailOptionAndShowsPath(t *testing.T) {
	root := t.TempDir()
	cfg := configForTest()
	logDir := filepath.Join(root, cfg.Sandbox.StateDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "device.log"), []byte("old\ncurrent\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := printLogs(&out, root, cfg, "device", logOptions{Tail: 1}); err != nil {
		t.Fatalf("printLogs returned error: %v", err)
	}
	text := out.String()
	if strings.Contains(text, "\nold\n") {
		t.Fatalf("tail output included old line:\n%s", text)
	}
	for _, want := range []string{"device.log", "showing last 1 lines", "current"} {
		if !strings.Contains(text, want) {
			t.Fatalf("tail output missing %q:\n%s", want, text)
		}
	}
}

func TestTailFileReturnsLastLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "device.log")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	text, err := tailFile(path, 2)
	if err != nil {
		t.Fatalf("tailFile returned error: %v", err)
	}
	if text != "two\nthree\n" {
		t.Fatalf("tailFile = %q", text)
	}
}
