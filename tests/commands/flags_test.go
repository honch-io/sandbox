package commands_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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

func TestSandboxFlagsListsAllVisibleCommandFlags(t *testing.T) {
	root := NewRootCommand(Dependencies{})
	root.SetArgs([]string{"--plain", "sandbox", "flags"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("flags returned error: %v\n%s", err, out.String())
	}
	plain := out.String()
	for _, flagName := range visibleCommandFlags(root) {
		if !strings.Contains(plain, flagName) {
			t.Fatalf("flags output missing visible flag %q:\n%s", flagName, plain)
		}
	}
}

func visibleCommandFlags(root *cobra.Command) []string {
	var flags []string
	collectFlagNames(&flags, root.PersistentFlags())
	walkVisibleCommands(root, func(cmd *cobra.Command) {
		collectFlagNames(&flags, cmd.NonInheritedFlags())
	})
	return flags
}

func walkVisibleCommands(cmd *cobra.Command, visit func(*cobra.Command)) {
	for _, child := range cmd.Commands() {
		if child.Hidden || child.Name() == "help" || child.Name() == "completion" {
			continue
		}
		visit(child)
		walkVisibleCommands(child, visit)
	}
}

func collectFlagNames(dst *[]string, flags *pflag.FlagSet) {
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Name == "help" {
			return
		}
		*dst = append(*dst, "--"+flag.Name)
	})
}
