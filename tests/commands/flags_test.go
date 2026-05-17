package commands_test

import (
	"bytes"
	"strings"
	"testing"
)

func TestSandboxFlagsShowsGroupedFlagInventory(t *testing.T) {
	root := NewRootCommand(Dependencies{})
	root.SetArgs([]string{"--plain", "sandbox", "flags"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("flags returned error: %v\n%s", err, out.String())
	}
	plain := out.String()
	for _, want := range []string{
		"Honch sandbox flags",
		"Global",
		"--plain",
		"Stack",
		"start --migrate",
		"Harness",
		"run --detach",
		"Network",
		"network --offline",
		"Setup",
		"qemu install --yes",
		"Inspect",
		"logs --tail",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("flags output missing %q:\n%s", want, plain)
		}
	}
}
