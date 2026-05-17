package ui

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	accent   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ea5924")).Bold(true)
	label    = lipgloss.NewStyle().Foreground(lipgloss.Color("#6f7895"))
	value    = lipgloss.NewStyle().Foreground(lipgloss.Color("#d8dee9")).Bold(true)
	neutral  = lipgloss.NewStyle().Foreground(lipgloss.Color("#8b93a7"))
	success  = lipgloss.NewStyle().Foreground(lipgloss.Color("#8bd17c")).Bold(true)
	failure  = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6b5f")).Bold(true)
	helpText = lipgloss.NewStyle().Foreground(lipgloss.Color("#a7adbb"))
	arrow    = lipgloss.NewStyle().Foreground(lipgloss.Color("#4ec9d8")).Bold(true)
	plain    = os.Getenv("NO_COLOR") != ""
)

type SilentError interface {
	error
	Silent() bool
}

type silentError struct {
	error
}

type Row struct {
	Key   string
	Value any
}

type Section struct {
	Name string
	Rows []Row
}

type CommandRow struct {
	Name        string
	Description string
}

type CommandSection struct {
	Name     string
	Commands []CommandRow
}

func SetPlain(enabled bool) {
	plain = enabled || os.Getenv("NO_COLOR") != ""
}

func IsPlain() bool {
	return plain
}

func NewSilentError(message string) error {
	return silentError{error: errors.New(message)}
}

func (silentError) Silent() bool {
	return true
}

func IsSilentError(err error) bool {
	var target SilentError
	return errors.As(err, &target)
}

func Heading(text string) string {
	return render(accent, text)
}

func Success(text string) string {
	return fmt.Sprintf("%s %s", render(success, "✓"), text)
}

func KeyValue(key string, value any) string {
	return FormatRow(key, value, 15)
}

func FormatKeyValues(title string, rows []Row) string {
	return FormatSections(title, []Section{{Rows: rows}})
}

// FormatSections is used for status-style output where scanability matters
// more than dense terminal tables.
func FormatSections(title string, sections []Section) string {
	width := 0
	for _, section := range sections {
		for _, row := range section.Rows {
			if len(row.Key) > width {
				width = len(row.Key)
			}
		}
	}
	if width < 7 {
		width = 7
	}
	if width < 18 {
		width = 18
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  ")
	b.WriteString(Heading(title))
	b.WriteString("\n\n")
	for i, section := range sections {
		if section.Name != "" {
			b.WriteString("     ")
			b.WriteString(render(label, section.Name))
			b.WriteString("\n")
		}
		indent := "     "
		if section.Name != "" {
			indent = "       "
		}
		for _, row := range section.Rows {
			b.WriteString(indent)
			b.WriteString(formatRow(row.Key, row.Value, width))
			b.WriteString("\n")
		}
		if section.Name != "" && i < len(sections)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func FormatRow(key string, rowValue any, width int) string {
	return formatRow(key, rowValue, width)
}

func formatRow(key string, rowValue any, width int) string {
	valueText := fmt.Sprint(rowValue)
	return fmt.Sprintf("%s %s   %s",
		render(label, fmt.Sprintf("%-*s", width, key)),
		render(arrow, "›"),
		render(valueStyle(rowValue), valueText),
	)
}

// FormatSectionsWrapped keeps doctor rows readable by breaking path-like
// entries onto a second line while leaving short status rows inline.
func FormatSectionsWrapped(title string, sections []Section) string {
	inlineWidth := 0
	for _, section := range sections {
		for _, row := range section.Rows {
			if isDoctorPathRow(row) {
				continue
			}
			if len(row.Key) > inlineWidth {
				inlineWidth = len(row.Key)
			}
		}
	}
	if inlineWidth < 7 {
		inlineWidth = 7
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  ")
	b.WriteString(Heading(title))
	b.WriteString("\n\n")
	for i, section := range sections {
		if section.Name != "" {
			b.WriteString("     ")
			b.WriteString(render(label, section.Name))
			b.WriteString("\n")
		}
		indent := "     "
		if section.Name != "" {
			indent = "       "
		}
		for _, row := range section.Rows {
			if isDoctorPathRow(row) {
				b.WriteString(indent)
				b.WriteString(render(label, row.Key))
				b.WriteString("\n")
				b.WriteString(indent)
				b.WriteString(" ")
				b.WriteString(render(arrow, "›"))
				b.WriteString("   ")
				b.WriteString(render(valueStyle(row.Value), fmt.Sprint(row.Value)))
				b.WriteString("\n")
				continue
			}
			b.WriteString(indent)
			b.WriteString(formatRow(row.Key, row.Value, inlineWidth))
			b.WriteString("\n")
		}
		if section.Name != "" && i < len(sections)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func FormatError(message string, rows []Row) string {
	var b strings.Builder
	b.WriteString(message)
	b.WriteString("\n")
	if len(rows) == 0 {
		return b.String()
	}
	b.WriteString("\n")
	width := 0
	for _, row := range rows {
		if len(row.Key) > width {
			width = len(row.Key)
		}
	}
	for _, row := range rows {
		b.WriteString("     ")
		b.WriteString(formatRow(row.Key, row.Value, width))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func FormatCommandHelp(title string, description string, usage string, flags []Row, commands []CommandRow) string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  ")
	b.WriteString(Heading(title))
	b.WriteString("\n\n")
	if description != "" {
		b.WriteString("    ")
		b.WriteString(render(helpText, description))
		b.WriteString("\n\n")
	}
	if usage != "" {
		b.WriteString("    ")
		b.WriteString(render(label, "Usage"))
		b.WriteString("\n")
		b.WriteString("      ")
		b.WriteString(render(helpText, usage))
		b.WriteString("\n\n")
	}
	if len(flags) > 0 {
		b.WriteString("    ")
		b.WriteString(render(label, "Flags"))
		b.WriteString("\n")
		width := 0
		for _, flag := range flags {
			if len(flag.Key) > width {
				width = len(flag.Key)
			}
		}
		for _, flag := range flags {
			b.WriteString("      ")
			b.WriteString(FormatRow(flag.Key, flag.Value, width))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if len(commands) > 0 {
		b.WriteString("    ")
		b.WriteString(render(label, "Commands"))
		b.WriteString("\n\n")
	}
	for _, command := range commands {
		b.WriteString("    ")
		b.WriteString(render(accent, command.Name))
		b.WriteString("\n")
		if command.Description != "" {
			b.WriteString("      ")
			b.WriteString(render(helpText, command.Description))
			b.WriteString("\n")
		}
	}
	return b.String()
}

// FormatGroupedCommandHelp keeps top-level command menus compact without
// losing the short descriptions contributors need while exploring the CLI.
func FormatGroupedCommandHelp(title string, description string, flow string, flagSections []Section, sections []CommandSection) string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  ")
	b.WriteString(Heading(title))
	b.WriteString("\n\n")
	if description != "" {
		b.WriteString("    ")
		b.WriteString(render(helpText, description))
		b.WriteString("\n\n")
	}
	if flow != "" {
		b.WriteString("    ")
		b.WriteString(render(label, "Flow"))
		b.WriteString("\n")
		b.WriteString("      ")
		b.WriteString(render(helpText, flow))
		b.WriteString("\n\n")
	}
	if len(flagSections) > 0 {
		b.WriteString("    ")
		b.WriteString(render(label, "Flags"))
		b.WriteString("\n")
		for i, section := range flagSections {
			if len(section.Rows) == 0 {
				continue
			}
			b.WriteString("      ")
			if section.Name != "" {
				b.WriteString(render(label, section.Name))
				b.WriteString("\n")
			}
			width := 0
			for _, flag := range section.Rows {
				if len(flag.Key) > width {
					width = len(flag.Key)
				}
			}
			if width < 7 {
				width = 7
			}
			indent := "        "
			if section.Name == "" {
				indent = "      "
			}
			for _, flag := range section.Rows {
				b.WriteString(indent)
				b.WriteString(FormatRow(flag.Key, flag.Value, width))
				b.WriteString("\n")
			}
			if i < len(flagSections)-1 {
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}
	width := commandNameWidth(sections)
	for i, section := range sections {
		if len(section.Commands) == 0 {
			continue
		}
		b.WriteString("    ")
		b.WriteString(render(label, section.Name))
		b.WriteString("\n")
		for _, command := range section.Commands {
			b.WriteString("      ")
			b.WriteString(formatCommandRow(command, width))
			b.WriteString("\n")
		}
		if i < len(sections)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func commandNameWidth(sections []CommandSection) int {
	width := 0
	for _, section := range sections {
		for _, command := range section.Commands {
			if len(command.Name) > width {
				width = len(command.Name)
			}
		}
	}
	if width < 6 {
		width = 6
	}
	return width
}

func formatCommandRow(command CommandRow, width int) string {
	return fmt.Sprintf("%s %s   %s",
		render(accent, fmt.Sprintf("%-*s", width, command.Name)),
		render(arrow, "›"),
		render(helpText, command.Description),
	)
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func StripANSI(text string) string {
	return ansiPattern.ReplaceAllString(text, "")
}

func render(style lipgloss.Style, text string) string {
	if plain {
		return text
	}
	return style.Render(text)
}

func isDoctorPathRow(row Row) bool {
	return looksLikeDoctorPath(fmt.Sprint(row.Key)) || looksLikeDoctorPath(fmt.Sprint(row.Value))
}

func looksLikeDoctorPath(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	if strings.Contains(text, "/") {
		return true
	}
	if strings.HasPrefix(text, "~") {
		return true
	}
	return strings.Contains(text, ":") && !strings.ContainsAny(text, " \t") && !strings.HasSuffix(text, ":")
}

func valueStyle(rowValue any) lipgloss.Style {
	text := strings.ToLower(fmt.Sprint(rowValue))
	switch {
	case text == "up", text == "ok", text == "clean", text == "online", text == "ready", strings.HasPrefix(text, "sandbox-"):
		return success
	case text == "dirty", text == "missing", text == "offline", text == "server-error", strings.HasPrefix(text, "down:"), strings.Contains(text, "failed"):
		return failure
	case text == "inactive", text == "none":
		return neutral
	default:
		return value
	}
}
