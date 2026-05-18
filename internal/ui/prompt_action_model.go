package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type promptActionButton struct {
	label  string
	action PromptAction
}

type actionModel struct {
	title    string
	prompt   string
	buttons  []promptActionButton
	selected int
}

func newActionModel(title string, prompt string, buttons []promptActionButton, selected int) actionModel {
	if len(buttons) == 0 {
		buttons = []promptActionButton{{label: "Exit", action: PromptActionExit}}
	}
	if selected < 0 || selected >= len(buttons) {
		selected = 0
	}
	return actionModel{title: title, prompt: prompt, buttons: buttons, selected: selected}
}

func (m actionModel) Init() tea.Cmd {
	return nil
}

func (m actionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "left", "h", "shift+tab":
			if m.selected > 0 {
				m.selected--
			}
			return m, nil
		case "right", "l", "tab":
			if m.selected < len(m.buttons)-1 {
				m.selected++
			}
			return m, nil
		case "b":
			if index := m.actionIndex(PromptActionBack); index >= 0 {
				m.selected = index
				return m, tea.Quit
			}
		case "q", "esc", "ctrl+c":
			if index := m.actionIndex(PromptActionExit); index >= 0 {
				m.selected = index
			}
			return m, tea.Quit
		case "n":
			if index := m.actionIndex(PromptActionNo); index >= 0 {
				m.selected = index
				return m, tea.Quit
			}
		case "y":
			if index := m.actionIndex(PromptActionYes); index >= 0 {
				m.selected = index
				return m, tea.Quit
			}
		case "enter":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m actionModel) View() string {
	var b strings.Builder
	b.WriteString("\n  ")
	b.WriteString(Heading(m.title))
	b.WriteString("\n\n    ")
	b.WriteString(render(helpText, m.prompt))
	b.WriteString("\n\n    ")
	for i, button := range m.buttons {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(confirmButton(button.label, i == m.selected))
	}
	b.WriteString("\n\n    ")
	b.WriteString(render(helpText, "tab/arrow selects, enter accepts, b goes back, q exits"))
	return b.String()
}

func (m actionModel) selectedAction() PromptAction {
	return m.buttons[m.selected].action
}

func (m actionModel) actionIndex(action PromptAction) int {
	for i, button := range m.buttons {
		if button.action == action {
			return i
		}
	}
	return -1
}

func confirmButton(label string, active bool) string {
	if active {
		return render(accent, "["+label+"]")
	}
	return render(helpText, "["+label+"]")
}
