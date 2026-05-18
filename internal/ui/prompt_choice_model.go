package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type choiceModel struct {
	title     string
	options   []PromptOption
	selected  int
	cancelled bool
}

func newChoiceModel(title string, options []PromptOption, selected int) choiceModel {
	if len(options) == 0 {
		options = []PromptOption{{Label: "Cancel"}}
	}
	if selected < 0 || selected >= len(options) {
		selected = 0
	}
	return choiceModel{title: title, options: options, selected: selected}
}

func (m choiceModel) Init() tea.Cmd {
	return nil
}

func (m choiceModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.cancelled = true
			return m, tea.Quit
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
			return m, nil
		case "down", "j":
			if m.selected < len(m.options)-1 {
				m.selected++
			}
			return m, nil
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			idx := int(msg.String()[0] - '1')
			if idx >= 0 && idx < len(m.options) {
				m.selected = idx
			}
			return m, nil
		case "enter":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m choiceModel) View() string {
	var b strings.Builder
	b.WriteString("\n  ")
	b.WriteString(Heading(m.title))
	b.WriteString("\n\n")
	for i, option := range m.options {
		prefix := "  "
		if i == m.selected {
			prefix = render(accent, "› ")
		}
		b.WriteString("    ")
		b.WriteString(prefix)
		b.WriteString(choiceLabel(option.Label, i))
		if option.Description != "" {
			b.WriteString("  ")
			b.WriteString(render(helpText, option.Description))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n    ")
	b.WriteString(render(helpText, "up/down or 1-9 to choose, enter to confirm, q to cancel"))
	return b.String()
}

func choiceLabel(label string, index int) string {
	return fmt.Sprintf("%d. %s", index+1, label)
}
