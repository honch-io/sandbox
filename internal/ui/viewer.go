package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type AppendMsg struct {
	Text string
}

type EventAppendMsg struct {
	Text string
}

type ProcessDoneMsg struct {
	Err error
}

type ProcessCommandFunc func(raw string) (string, error)

var (
	commandHintStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#a7adbb"))
	commandCursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#ea5924")).Bold(true)
	commandPreviewStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#8b93a7"))
)

type TextViewerModel struct {
	title    string
	footer   string
	content  string
	follow   bool
	ready    bool
	viewport viewport.Model
}

func NewTextViewerModel(title string, content string, footer string) TextViewerModel {
	return TextViewerModel{
		title:   title,
		content: content,
		footer:  footer,
		follow:  true,
	}
}

func (m TextViewerModel) Init() tea.Cmd {
	return nil
}

func (m TextViewerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewport.Width = msg.Width
		height := msg.Height - 6
		if height < 1 {
			height = 1
		}
		m.viewport.Height = height
		m.viewport.SetContent(m.content)
		if m.follow {
			m.viewport.GotoBottom()
		}
		m.ready = true
		return m, nil
	case AppendMsg:
		if msg.Text == "" {
			return m, nil
		}
		m.content += msg.Text
		m.viewport.SetContent(m.content)
		if m.follow {
			m.viewport.GotoBottom()
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case " ":
			m.follow = !m.follow
			if m.follow {
				m.viewport.GotoBottom()
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m TextViewerModel) View() string {
	var b strings.Builder
	b.WriteString("\n  ")
	b.WriteString(Heading(m.title))
	b.WriteString("\n\n")
	if m.ready {
		b.WriteString(m.viewport.View())
	} else if m.content != "" {
		b.WriteString(indentBlock(m.content, "  "))
	} else {
		b.WriteString("  loading...\n")
	}
	if m.footer != "" {
		b.WriteString("\n  ")
		b.WriteString(render(helpText, m.footer))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func indentBlock(text string, prefix string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n") + "\n"
}

func (m TextViewerModel) WithContent(content string) TextViewerModel {
	m.content = content
	m.viewport.SetContent(content)
	if m.follow {
		m.viewport.GotoBottom()
	}
	return m
}

func ViewerFooter() string {
	return "q quit  space follow  arrows/page scroll"
}

func ViewerTitle(title string, suffix string) string {
	if suffix == "" {
		return title
	}
	return fmt.Sprintf("%s (%s)", title, suffix)
}
