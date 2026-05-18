package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type confirmModel struct {
	prompt string
	choice int
}

func newConfirmModel(prompt string) confirmModel {
	return confirmModel{prompt: prompt, choice: 1}
}

func (m confirmModel) Init() tea.Cmd {
	return nil
}

func (m confirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y", "right", "l":
			m.choice = 1
			return m, tea.Quit
		case "n", "N", "left", "h", "esc", "q":
			m.choice = 0
			return m, tea.Quit
		case "enter":
			return m, tea.Quit
		case "tab", "up", "down":
			if m.choice == 0 {
				m.choice = 1
			} else {
				m.choice = 0
			}
			return m, nil
		}
	}
	return m, nil
}

func (m confirmModel) View() string {
	var b strings.Builder
	b.WriteString("\n  ")
	b.WriteString(Heading("Confirm"))
	b.WriteString("\n\n    ")
	b.WriteString(render(helpText, m.prompt))
	b.WriteString("\n\n    ")
	b.WriteString(confirmButton("No", m.choice == 0))
	b.WriteString("  ")
	b.WriteString(confirmButton("Yes", m.choice == 1))
	b.WriteString("\n\n    ")
	b.WriteString(render(helpText, "enter accepts, y/n and esc work too"))
	return b.String()
}

func (m confirmModel) confirmed() bool {
	return m.choice == 1
}
