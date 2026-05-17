package commands_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImagesListShowsLongImageNames(t *testing.T) {
	rootDir := t.TempDir()
	binDir := filepath.Join(rootDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	docker := filepath.Join(binDir, "docker")
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(docker, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir)

	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "images", "list"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("images list returned error: %v\n%s", err, out.String())
	}

	longImage := "gcr.io/google.com/cloudsdktool/cloud-sdk:emulators"
	if !strings.Contains(out.String(), longImage) {
		t.Fatalf("long image name was not shown in full:\n%s", out.String())
	}
}
