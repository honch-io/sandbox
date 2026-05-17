package commands

import (
	"fmt"
	"sort"
	"strings"

	"github.com/honch/sdk/tools/sandbox/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
				"start -> run esp-idf --detach -> track -> flush -> events list -> stop",
				sandboxHelpFlagSections(cmd),
				sandboxHelpSections(),
			))
			return
		}
		if cmd.CommandPath() == "honch" {
			_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.FormatGroupedCommandHelp(
				helpTitle(cmd),
				cmd.Short,
				"",
				rootHelpFlagSections(cmd),
				[]ui.CommandSection{
					{
						Name:     "Tools",
						Commands: visibleCommands(cmd),
					},
				},
			))
			return
		}
		_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.FormatCommandHelp(helpTitle(cmd), cmd.Short, commandUsage(cmd), commandFlagRows(cmd), visibleCommands(cmd)))
	})
	for _, child := range root.Commands() {
		installHelp(child)
	}
}

func rootHelpFlagSections(cmd *cobra.Command) []ui.Section {
	return []ui.Section{
		{Name: "Global", Rows: flagRows(cmd.Root().PersistentFlags(), "")},
	}
}

func sandboxHelpFlagSections(cmd *cobra.Command) []ui.Section {
	root := cmd.Root()
	sections := []ui.Section{
		{Name: "Global", Rows: flagRows(root.PersistentFlags(), "")},
	}
	for _, group := range []struct {
		name string
		path []string
	}{
		{name: "Stack", path: []string{"sandbox", "start"}},
		{name: "Harness", path: []string{"sandbox", "run"}},
		{name: "Network", path: []string{"sandbox", "network"}},
		{name: "Setup", path: []string{"sandbox", "setup"}},
		{name: "Setup", path: []string{"sandbox", "qemu", "install"}},
		{name: "Harness", path: []string{"sandbox", "battery"}},
		{name: "Harness", path: []string{"sandbox", "track"}},
		{name: "Inspect", path: []string{"sandbox", "logs"}},
	} {
		if rows := commandFlagSection(root, root.CommandPath()+" sandbox", group.name, group.path); len(rows.Rows) > 0 {
			sections = appendFlagSection(sections, rows)
		}
	}
	return sections
}

func commandFlagSection(root *cobra.Command, basePath string, name string, path []string) ui.Section {
	cmd, _, _ := root.Find(path)
	if cmd == nil {
		return ui.Section{}
	}
	return ui.Section{
		Name: name,
		Rows: flagRows(cmd.NonInheritedFlags(), commandPrefix(cmd, basePath)),
	}
}

func appendFlagSection(sections []ui.Section, next ui.Section) []ui.Section {
	for i := range sections {
		if sections[i].Name == next.Name {
			sections[i].Rows = append(sections[i].Rows, next.Rows...)
			return sections
		}
	}
	return append(sections, next)
}

func flagRows(flags *pflag.FlagSet, prefix string) []ui.Row {
	rows := []ui.Row{}
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden {
			return
		}
		name := "--" + flag.Name
		if flag.Shorthand != "" {
			name = "-" + flag.Shorthand + ", " + name
		}
		if prefix != "" {
			name = prefix + " " + name
		}
		rows = append(rows, ui.Row{Key: name, Value: flag.Usage})
	})
	return rows
}

func commandPrefix(cmd *cobra.Command, basePath string) string {
	prefix := strings.TrimPrefix(cmd.CommandPath(), basePath+" ")
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return cmd.Name()
	}
	return prefix
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
				{Name: "adapters", Description: "Inspect adapter configs"},
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
			Name: "Setup",
			Commands: []ui.CommandRow{
				{Name: "doctor", Description: "Check local prerequisites"},
				{Name: "setup", Description: "Install supported prerequisites"},
				{Name: "images", Description: "Pull required Docker images"},
				{Name: "qemu", Description: "Manage ESP-IDF QEMU tooling"},
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

func visibleChildCommands(cmd *cobra.Command) []*cobra.Command {
	children := cmd.Commands()
	sort.Slice(children, func(i, j int) bool {
		return children[i].Name() < children[j].Name()
	})
	result := make([]*cobra.Command, 0, len(children))
	for _, child := range children {
		if child.Hidden || child.Name() == "help" || child.Name() == "completion" {
			continue
		}
		result = append(result, child)
	}
	return result
}

func helpTitle(cmd *cobra.Command) string {
	if cmd.CommandPath() == "" {
		return cmd.Use
	}
	return cmd.CommandPath()
}

func visibleCommands(cmd *cobra.Command) []ui.CommandRow {
	children := visibleChildCommands(cmd)
	rows := make([]ui.CommandRow, 0, len(children))
	for _, child := range children {
		rows = append(rows, ui.CommandRow{Name: child.Name(), Description: child.Short})
	}
	return rows
}

func commandUsage(cmd *cobra.Command) string {
	if len(visibleCommands(cmd)) > 0 {
		return ""
	}
	return cmd.UseLine()
}

func commandFlagRows(cmd *cobra.Command) []ui.Row {
	if len(visibleCommands(cmd)) > 0 {
		return nil
	}
	rows := flagRows(cmd.InheritedFlags(), "")
	rows = append(rows, flagRows(cmd.NonInheritedFlags(), "")...)
	return rows
}
