package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
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

func ParseProcessCommand(raw string) (string, map[string]any, string, error) {
	action, fields, _, status, err := processCommandSpec(raw)
	if err != nil {
		return "", nil, "", err
	}
	switch action {
	case "flush":
		return action, nil, status, nil
	case "reset":
		return action, nil, status, nil
	case "battery":
		level := fields["level"].(int)
		return action, fields, fmt.Sprintf("battery control has been sent: %d", level), nil
	case "track":
		return action, fields, status, nil
	default:
		return "", nil, "", fmt.Errorf("unknown command %q", action)
	}
}

func ProcessCommandPreview(raw string) (string, error) {
	action, _, preview, _, err := processCommandSpec(raw)
	if err != nil {
		return "", err
	}
	switch action {
	case "flush":
		return "will send: flush", nil
	case "reset":
		return "will send: reset", nil
	case "battery":
		return preview, nil
	case "track":
		return preview, nil
	default:
		return "", fmt.Errorf("unknown command %q", action)
	}
}

func processCommandSpec(raw string) (string, map[string]any, string, string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil, "", "", errors.New("missing command")
	}

	fields := strings.Fields(trimmed)
	switch fields[0] {
	case "flush":
		if len(fields) != 1 {
			return "", nil, "", "", errors.New("flush does not take arguments")
		}
		return "flush", nil, "will send: flush", "flush control has been sent", nil
	case "reset":
		if len(fields) != 1 {
			return "", nil, "", "", errors.New("reset does not take arguments")
		}
		return "reset", nil, "will send: reset", "reset control has been sent", nil
	case "battery":
		if len(fields) < 2 {
			return "", nil, "", "", errors.New("missing battery level")
		}
		levelToken := strings.TrimPrefix(fields[1], "level=")
		levelToken = strings.TrimPrefix(levelToken, "level:")
		level, err := strconv.Atoi(levelToken)
		if err != nil || level < 0 || level > 100 {
			return "", nil, "", "", errors.New("battery level must be between 0 and 100")
		}
		return "battery", map[string]any{"level": level}, fmt.Sprintf("will send: battery %d", level), fmt.Sprintf("battery control has been sent: %d", level), nil
	case "track":
		if len(fields) < 2 {
			return "", nil, "", "", errors.New("missing event name")
		}
		event := fields[1]
		properties := map[string]any{}
		if len(fields) == 2 {
			preview := fmt.Sprintf("will send: track %s {}", event)
			status := fmt.Sprintf("track control has been sent: %s", event)
			return "track", map[string]any{"event": event, "properties": properties}, preview, status, nil
		}
		remainder := strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))
		remainder = strings.TrimSpace(strings.TrimPrefix(remainder, event))
		if strings.HasPrefix(remainder, "{") {
			if err := json.Unmarshal([]byte(remainder), &properties); err != nil || properties == nil {
				return "", nil, "", "", errors.New("properties must be valid JSON")
			}
		} else {
			props, err := parseShorthandProperties(fields[2:])
			if err != nil {
				return "", nil, "", "", err
			}
			properties = props
		}
		preview := fmt.Sprintf("will send: track %s %s", event, renderJSONObject(properties))
		status := fmt.Sprintf("track control has been sent: %s", event)
		return "track", map[string]any{"event": event, "properties": properties}, preview, status, nil
	default:
		return "", nil, "", "", fmt.Errorf("unknown command %q", fields[0])
	}
}

func parseShorthandProperties(tokens []string) (map[string]any, error) {
	props := map[string]any{}
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		key, value, ok := strings.Cut(token, "=")
		if !ok {
			key, value, ok = strings.Cut(token, ":")
		}
		if !ok || strings.TrimSpace(key) == "" {
			return nil, errors.New("properties must use key=value shorthand or JSON")
		}
		props[strings.TrimSpace(key)] = parsePropertyValue(strings.TrimSpace(value))
	}
	return props, nil
}

func parsePropertyValue(value string) any {
	if value == "" {
		return ""
	}
	if json.Valid([]byte(value)) {
		var parsed any
		if err := json.Unmarshal([]byte(value), &parsed); err == nil {
			return parsed
		}
	}
	if i, err := strconv.Atoi(value); err == nil {
		return i
	}
	if strings.EqualFold(value, "true") {
		return true
	}
	if strings.EqualFold(value, "false") {
		return false
	}
	return value
}

func renderJSONObject(values map[string]any) string {
	if len(values) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		keyJSON, err := json.Marshal(key)
		if err != nil {
			return "{}"
		}
		valueJSON, err := json.Marshal(values[key])
		if err != nil {
			return "{}"
		}
		b.WriteString(string(keyJSON))
		b.WriteByte(':')
		b.WriteString(string(valueJSON))
	}
	b.WriteByte('}')
	return b.String()
}

func commandInputStatus(err error) string {
	switch err.Error() {
	case "missing command":
		return "examples: battery 4, track camera.motion zone=porch"
	case "missing battery level":
		return "enter a battery level, e.g. 4"
	case "battery level must be between 0 and 100":
		return "battery level must be between 0 and 100"
	case "missing event name":
		return "enter an event name, e.g. camera.motion"
	case "flush does not take arguments":
		return "flush takes no arguments"
	case "reset does not take arguments":
		return "reset takes no arguments"
	case "properties must use key=value shorthand or JSON":
		return "use key=value shorthand or JSON"
	case "properties must be valid JSON":
		return "JSON is malformed; try zone=porch instead"
	case "unknown command":
		return "unknown command"
	default:
		if strings.HasPrefix(err.Error(), "unknown command ") {
			return "unknown command"
		}
		return err.Error()
	}
}
