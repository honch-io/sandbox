package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type textModel struct {
	prompt       string
	defaultValue string
	value        string
	cancelled    bool
}

func newTextModel(prompt string, defaultValue string) textModel {
	return textModel{prompt: prompt, defaultValue: defaultValue}
}

func (m textModel) Init() tea.Cmd {
	return nil
}

func (m textModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "enter":
			return m, tea.Quit
		case "backspace", "ctrl+h":
			if m.value != "" {
				runes := []rune(m.value)
				m.value = string(runes[:len(runes)-1])
			}
			return m, nil
		case "ctrl+u":
			m.value = ""
			return m, nil
		}
		switch msg.Type {
		case tea.KeyRunes:
			m.value += string(msg.Runes)
		case tea.KeySpace:
			m.value += " "
		}
		return m, nil
	}
	return m, nil
}

func (m textModel) View() string {
	var b strings.Builder
	b.WriteString("\n  ")
	b.WriteString(Heading("Input"))
	b.WriteString("\n\n    ")
	b.WriteString(render(helpText, m.prompt))
	if m.defaultValue != "" {
		b.WriteString(render(helpText, " (blank keeps default)"))
	}
	b.WriteString("\n\n    ")
	if m.value != "" {
		b.WriteString(render(value, m.value))
	} else if m.defaultValue != "" {
		b.WriteString(render(helpText, m.defaultValue))
	}
	b.WriteString(render(accent, "█"))
	b.WriteString("\n\n    ")
	b.WriteString(render(helpText, "enter accepts, esc cancels, ctrl+u clears"))
	return b.String()
}
