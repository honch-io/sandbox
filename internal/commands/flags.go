package commands

import (
	"fmt"

	"github.com/honch/sdk/tools/sandbox/internal/ui"
	"github.com/spf13/cobra"
)

func newFlagsCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "flags",
		Short: "Show sandbox command flags",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = deps
			root := cmd.Root()
			_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.FormatSections("Honch sandbox flags", sandboxFlagSections(root)))
			return nil
		},
	}
}

func sandboxFlagSections(root *cobra.Command) []ui.Section {
	sections := []ui.Section{
		{Name: "Global", Rows: flagRows(root.PersistentFlags(), "")},
	}

	for _, group := range []struct {
		name  string
		paths [][]string
	}{
		{
			name: "Stack",
			paths: [][]string{
				{"sandbox", "start"},
			},
		},
		{
			name: "Harness",
			paths: [][]string{
				{"sandbox", "run"},
				{"sandbox", "battery"},
				{"sandbox", "track"},
			},
		},
		{
			name: "Network",
			paths: [][]string{
				{"sandbox", "network"},
			},
		},
		{
			name: "Setup",
			paths: [][]string{
				{"sandbox", "setup"},
				{"sandbox", "qemu", "install"},
			},
		},
		{
			name: "Inspect",
			paths: [][]string{
				{"sandbox", "logs"},
			},
		},
	} {
		rows := []ui.Row{}
		for _, path := range group.paths {
			rows = append(rows, commandFlagRowsForPath(root, path)...)
		}
		if len(rows) > 0 {
			sections = append(sections, ui.Section{Name: group.name, Rows: rows})
		}
	}
	return sections
}

func commandFlagRowsForPath(root *cobra.Command, path []string) []ui.Row {
	cmd, _, _ := root.Find(path)
	if cmd == nil {
		return nil
	}
	return flagRows(cmd.NonInheritedFlags(), commandPrefix(cmd, root.CommandPath()+" sandbox"))
}
