package commands

import (
	"fmt"
	"sort"

	"github.com/honch/sdk/tools/sandbox/internal/ui"
	"github.com/spf13/cobra"
)

func installHelp(root *cobra.Command) {
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if flag := cmd.Root().PersistentFlags().Lookup("plain"); flag != nil && flag.Changed {
			ui.SetPlain(flag.Value.String() == "true")
		}
		if cmd.CommandPath() == "honch sandbox" {
			_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.FormatGroupedCommandHelp(
				helpTitle(cmd),
				cmd.Short,
				"start -> run c-core --detach -> track -> flush -> events list -> stop",
				sandboxHelpSections(),
			))
			return
		}
		if cmd.CommandPath() == "honch" {
			_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.FormatGroupedCommandHelp(
				helpTitle(cmd),
				cmd.Short,
				"",
				[]ui.CommandSection{
					{
						Name:     "Tools",
						Commands: visibleCommands(cmd),
					},
				},
			))
			return
		}
		_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.FormatCommandHelp(helpTitle(cmd), cmd.Short, visibleCommands(cmd)))
	})
	for _, child := range root.Commands() {
		installHelp(child)
	}
}

func sandboxHelpSections() []ui.CommandSection {
	return []ui.CommandSection{
		{
			Name: "Stack",
			Commands: []ui.CommandRow{
				{Name: "start", Description: "Start the local Honch stack"},
				{Name: "stop", Description: "Stop sandbox services"},
				{Name: "status", Description: "Show health and session state"},
				{Name: "update", Description: "Fast-forward clean sibling repos"},
			},
		},
		{
			Name: "Harness",
			Commands: []ui.CommandRow{
				{Name: "run", Description: "Build and run an SDK harness"},
				{Name: "battery", Description: "Set harness battery level"},
				{Name: "track", Description: "Emit a custom event"},
				{Name: "flush", Description: "Flush queued events"},
				{Name: "reset", Description: "Run SDK reset behavior"},
			},
		},
		{
			Name: "Network",
			Commands: []ui.CommandRow{
				{Name: "network", Description: "Set proxy mode"},
			},
		},
		{
			Name: "Inspect",
			Commands: []ui.CommandRow{
				{Name: "events", Description: "Query ClickHouse events"},
				{Name: "logs", Description: "Print sandbox logs"},
				{Name: "scenario", Description: "Run a YAML scenario"},
			},
		},
	}
}

func helpTitle(cmd *cobra.Command) string {
	if cmd.CommandPath() == "" {
		return cmd.Use
	}
	return cmd.CommandPath()
}

func visibleCommands(cmd *cobra.Command) []ui.CommandRow {
	children := cmd.Commands()
	sort.Slice(children, func(i, j int) bool {
		return children[i].Name() < children[j].Name()
	})
	rows := make([]ui.CommandRow, 0, len(children))
	for _, child := range children {
		if child.Hidden || child.Name() == "help" || child.Name() == "completion" {
			continue
		}
		rows = append(rows, ui.CommandRow{Name: child.Name(), Description: child.Short})
	}
	return rows
}
