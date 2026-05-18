package ui

import (
	"context"
	"errors"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type ProcessViewerModel struct {
	TextViewerModel
	cancel         context.CancelFunc
	done           <-chan error
	command        ProcessCommandFunc
	Err            error
	status         string
	input          []rune
	editing        bool
	showEvents     bool
	eventsFollow   bool
	eventsContent  string
	eventsReady    bool
	eventsViewport viewport.Model
}

func NewProcessViewerModel(title string, content string, footer string, done <-chan error, cancel context.CancelFunc, command ProcessCommandFunc) ProcessViewerModel {
	return ProcessViewerModel{
		TextViewerModel: NewTextViewerModel(title, content, footer),
		cancel:          cancel,
		done:            done,
		command:         command,
		eventsFollow:    true,
	}
}

func (m ProcessViewerModel) Init() tea.Cmd {
	return waitForProcessResult(m.done)
}

func (m ProcessViewerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ProcessDoneMsg:
		m.Err = msg.Err
		m.content = strings.TrimRight(m.content, "\n")
		if m.content != "" {
			m.content += "\n\n"
		}
		switch {
		case msg.Err == nil:
			m.content += "  " + Success("run complete") + "\n"
		case errors.Is(msg.Err, context.Canceled):
			m.content += "  " + render(failure, "×") + " run cancelled\n"
		default:
			m.content += "  " + render(failure, "×") + " run failed\n"
		}
		m.viewport.SetContent(m.content)
		if m.follow {
			m.viewport.GotoBottom()
		}
		return m, tea.Quit
	case EventAppendMsg:
		if msg.Text == "" {
			return m, nil
		}
		m.eventsContent += msg.Text
		m.eventsViewport.SetContent(m.eventsContent)
		if m.eventsFollow {
			m.eventsViewport.GotoBottom()
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.viewport.Width = msg.Width
		m.eventsViewport.Width = msg.Width
		height := processViewerViewportHeight(msg.Height, m.editing)
		if height < 1 {
			height = 1
		}
		m.viewport.Height = height
		m.viewport.SetContent(m.content)
		m.eventsViewport.Height = height
		m.eventsViewport.SetContent(m.eventsContent)
		if m.follow {
			m.viewport.GotoBottom()
		}
		if m.eventsFollow {
			m.eventsViewport.GotoBottom()
		}
		m.ready = true
		m.eventsReady = true
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
		return m.handleKeyMsg(msg)
	}
	model, cmd := m.TextViewerModel.Update(msg)
	updated, ok := model.(TextViewerModel)
	if ok {
		m.TextViewerModel = updated
	}
	return m, cmd
}

func (m ProcessViewerModel) View() string {
	var b strings.Builder
	b.WriteString("\n  ")
	b.WriteString(Heading(m.viewerTitle()))
	b.WriteString("\n\n")
	if m.showEvents {
		if m.eventsReady {
			b.WriteString(m.eventsViewport.View())
		} else if m.eventsContent != "" {
			b.WriteString(indentBlock(m.eventsContent, "  "))
		} else {
			b.WriteString("  No events yet...\n")
		}
	} else if m.ready {
		b.WriteString(m.viewport.View())
	} else if m.content != "" {
		b.WriteString(indentBlock(m.content, "  "))
	} else {
		b.WriteString("  loading...\n")
	}
	b.WriteString("\n")
	b.WriteString(m.bottomBarView())
	b.WriteString("\n")
	return strings.TrimRight(b.String(), "\n")
}

func (m ProcessViewerModel) viewerTitle() string {
	if m.showEvents {
		return ViewerTitle(m.title, "events tail")
	}
	return m.title
}

func (m ProcessViewerModel) bottomBarView() string {
	if m.showEvents {
		var b strings.Builder
		b.WriteString("  ")
		b.WriteString(render(commandHintStyle, "tab run  space follow  q quit  arrows/page scroll"))
		if m.status != "" {
			b.WriteString("\n  ")
			b.WriteString(render(commandPreviewStyle, m.status))
		}
		return b.String()
	}
	if m.editing {
		return m.commandBarView()
	}
	var b strings.Builder
	b.WriteString("  ")
	b.WriteString(render(commandHintStyle, "b battery  t track  f flush  r reset  tab events  : type a command"))
	if m.status != "" {
		b.WriteString("\n  ")
		b.WriteString(render(commandPreviewStyle, m.status))
	}
	return b.String()
}

func (m ProcessViewerModel) WithEventsContent(content string) ProcessViewerModel {
	m.eventsContent = content
	m.eventsViewport.SetContent(content)
	if m.eventsFollow {
		m.eventsViewport.GotoBottom()
	}
	return m
}

func (m ProcessViewerModel) commandBarView() string {
	var b strings.Builder
	b.WriteString("  ")
	b.WriteString(render(commandHintStyle, "command"))
	b.WriteString("\n  ")
	display := "> " + string(m.input) + render(commandCursorStyle, "▌")
	b.WriteString(render(commandPreviewStyle, display))
	b.WriteString("\n")
	if m.status != "" {
		b.WriteString("  ")
		b.WriteString(render(commandPreviewStyle, m.status))
	}
	return strings.TrimRight(b.String(), "\n")
}

func waitForProcessResult(done <-chan error) tea.Cmd {
	return func() tea.Msg {
		return ProcessDoneMsg{Err: <-done}
	}
}

func (m ProcessViewerModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.editing {
		switch msg.String() {
		case "ctrl+c":
			if m.cancel != nil {
				m.cancel()
			}
			m.Err = context.Canceled
			return m, tea.Quit
		case "tab":
			m.editing = false
			m.input = nil
			m.status = ""
			m.showEvents = !m.showEvents
			if m.showEvents && m.eventsFollow {
				m.eventsViewport.GotoBottom()
			}
			if !m.showEvents && m.follow {
				m.viewport.GotoBottom()
			}
			return m, nil
		case "esc":
			m.editing = false
			m.input = nil
			m.status = ""
			return m, nil
		case "backspace", "delete", "ctrl+h":
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
			m.refreshCommandStatus()
			return m, nil
		case "enter":
			raw := strings.TrimSpace(string(m.input))
			if raw == "" {
				m.editing = false
				m.input = nil
				return m, nil
			}
			if m.command == nil {
				m.status = "command entry is unavailable"
				m.editing = false
				m.input = nil
				return m, nil
			}
			status, err := m.command(raw)
			if err != nil {
				m.status = err.Error()
			} else {
				m.status = status
			}
			m.editing = false
			m.input = nil
			return m, nil
		default:
			text := msg.String()
			if len(text) == 1 && text[0] >= 32 {
				m.input = append(m.input, []rune(text)...)
				m.refreshCommandStatus()
			}
			return m, nil
		}
	}

	switch msg.String() {
	case "ctrl+c", "q":
		if m.cancel != nil {
			m.cancel()
		}
		m.Err = context.Canceled
		return m, tea.Quit
	case "esc":
		if len(m.input) > 0 || m.editing {
			m.editing = false
			m.input = nil
			m.status = ""
			return m, nil
		}
		if m.cancel != nil {
			m.cancel()
		}
		m.Err = context.Canceled
		return m, tea.Quit
	case " ":
		if m.showEvents {
			m.eventsFollow = !m.eventsFollow
			if m.eventsFollow {
				m.eventsViewport.GotoBottom()
			}
			return m, nil
		}
		m.follow = !m.follow
		if m.follow {
			m.viewport.GotoBottom()
		}
		return m, nil
	case "tab":
		m.showEvents = !m.showEvents
		if m.showEvents && m.eventsFollow {
			m.eventsViewport.GotoBottom()
		}
		if !m.showEvents && m.follow {
			m.viewport.GotoBottom()
		}
		return m, nil
	case ":":
		m.showEvents = false
		m.editing = true
		m.input = nil
		m.status = "examples: battery 4 or track camera.motion zone=porch"
		return m, nil
	case "b":
		m.showEvents = false
		m.editing = true
		m.input = []rune("battery 4")
		m.status = "press enter to send battery 4"
		return m, nil
	case "t":
		m.showEvents = false
		m.editing = true
		m.input = []rune("track camera.motion example=")
		m.status = "add a value after example="
		return m, nil
	case "f":
		m.showEvents = false
		if m.command != nil {
			status, err := m.command("flush")
			if err != nil {
				m.status = err.Error()
			} else {
				m.status = status
			}
		}
		return m, nil
	case "r":
		m.showEvents = false
		if m.command != nil {
			status, err := m.command("reset")
			if err != nil {
				m.status = err.Error()
			} else {
				m.status = status
			}
		}
		return m, nil
	}

	model, cmd := m.TextViewerModel.Update(msg)
	updated, ok := model.(TextViewerModel)
	if ok {
		m.TextViewerModel = updated
	}
	return m, cmd
}

func processViewerViewportHeight(totalHeight int, editing bool) int {
	reserved := 4
	if editing {
		reserved = 7
	}
	height := totalHeight - reserved
	if height < 1 {
		return 1
	}
	return height
}

func (m *ProcessViewerModel) refreshCommandStatus() {
	if !m.editing {
		return
	}
	preview, err := ProcessCommandPreview(string(m.input))
	if err != nil {
		m.status = commandInputStatus(err)
		return
	}
	m.status = preview
}
