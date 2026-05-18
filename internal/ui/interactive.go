package ui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
)

var ErrPromptCancelled = errors.New("prompt cancelled")

type PromptAction int

const (
	PromptActionNo PromptAction = iota
	PromptActionYes
	PromptActionBack
	PromptActionExit
	PromptActionContinue
)

type PromptOption struct {
	Label       string
	Description string
}

type PromptSession struct {
	in     io.Reader
	out    io.Writer
	reader *bufio.Reader
}

func NewPromptSession(in io.Reader, out io.Writer) *PromptSession {
	return &PromptSession{
		in:     in,
		out:    out,
		reader: bufio.NewReader(in),
	}
}

func (p *PromptSession) Confirm(prompt string) (bool, error) {
	return p.ConfirmDefault(prompt, false)
}

func (p *PromptSession) ConfirmDefault(prompt string, defaultValue bool) (bool, error) {
	if IsInteractive(p.in, p.out) && !plain {
		return PromptConfirm(p.in, p.out, prompt)
	}
	suffix := " [y/N] "
	if defaultValue {
		suffix = " [Y/n] "
	}
	_, _ = fmt.Fprint(p.out, prompt+suffix)
	answer, err := p.readLine()
	if err != nil {
		return false, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer == "" {
		return defaultValue, nil
	}
	return answer == "y" || answer == "yes", nil
}

func (p *PromptSession) ContinueOrExit(prompt string) (PromptAction, error) {
	if IsInteractive(p.in, p.out) && !plain {
		return PromptContinueOrExit(p.in, p.out, prompt)
	}
	_, _ = fmt.Fprint(p.out, prompt+" [Enter/q] ")
	answer, err := p.readLine()
	if err != nil {
		return PromptActionExit, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer == "q" || answer == "quit" || answer == "exit" {
		return PromptActionExit, nil
	}
	return PromptActionContinue, nil
}

func (p *PromptSession) ConfirmNavigate(prompt string, defaultValue bool, canBack bool) (PromptAction, error) {
	if IsInteractive(p.in, p.out) && !plain {
		return PromptConfirmNavigate(p.in, p.out, prompt, defaultValue, canBack)
	}
	suffix := " [y/N/q] "
	if canBack {
		suffix = " [y/N/b/q] "
	}
	if defaultValue {
		suffix = " [Y/n/q] "
		if canBack {
			suffix = " [Y/n/b/q] "
		}
	}
	_, _ = fmt.Fprint(p.out, prompt+suffix)
	answer, err := p.readLine()
	if err != nil {
		return PromptActionExit, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	switch answer {
	case "":
		if defaultValue {
			return PromptActionYes, nil
		}
		return PromptActionNo, nil
	case "y", "yes":
		return PromptActionYes, nil
	case "b", "back":
		if canBack {
			return PromptActionBack, nil
		}
		return PromptActionNo, nil
	case "q", "quit", "exit":
		return PromptActionExit, nil
	default:
		return PromptActionNo, nil
	}
}

func (p *PromptSession) Text(prompt string, defaultValue string) (string, error) {
	if IsInteractive(p.in, p.out) && !plain {
		return PromptText(p.in, p.out, prompt, defaultValue)
	}
	if defaultValue != "" {
		prompt = fmt.Sprintf("%s [%s] ", prompt, defaultValue)
	} else {
		prompt = prompt + " "
	}
	_, _ = fmt.Fprint(p.out, prompt)
	answer, err := p.readLine()
	if err != nil {
		return "", err
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return defaultValue, nil
	}
	return answer, nil
}

func (p *PromptSession) readLine() (string, error) {
	answer, err := p.reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return answer, nil
}

func IsInteractive(in io.Reader, out io.Writer) bool {
	inFile, ok := in.(*os.File)
	if !ok || inFile == nil || !isatty.IsTerminal(inFile.Fd()) {
		return false
	}
	outFile, ok := out.(*os.File)
	if !ok || outFile == nil || !isatty.IsTerminal(outFile.Fd()) {
		return false
	}
	return true
}

func PromptConfirm(in io.Reader, out io.Writer, prompt string) (bool, error) {
	if !IsInteractive(in, out) || plain {
		return promptConfirmText(in, out, prompt)
	}
	model := newConfirmModel(prompt)
	program := tea.NewProgram(model, tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen())
	final, err := program.Run()
	if err != nil {
		if errors.Is(err, tea.ErrInterrupted) {
			return false, ErrPromptCancelled
		}
		return false, err
	}
	result, ok := final.(confirmModel)
	if !ok {
		return false, fmt.Errorf("unexpected prompt model %T", final)
	}
	return result.confirmed(), nil
}

func PromptText(in io.Reader, out io.Writer, prompt string, defaultValue string) (string, error) {
	if !IsInteractive(in, out) || plain {
		return NewPromptSession(in, out).Text(prompt, defaultValue)
	}
	model := newTextModel(prompt, defaultValue)
	program := tea.NewProgram(model, tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen())
	final, err := program.Run()
	if err != nil {
		if errors.Is(err, tea.ErrInterrupted) {
			return "", ErrPromptCancelled
		}
		return "", err
	}
	result, ok := final.(textModel)
	if !ok {
		return "", fmt.Errorf("unexpected prompt model %T", final)
	}
	if result.cancelled {
		return "", ErrPromptCancelled
	}
	value := strings.TrimSpace(result.value)
	if value == "" {
		return defaultValue, nil
	}
	return value, nil
}

func PromptChoice(in io.Reader, out io.Writer, title string, options []PromptOption, selected int) (int, error) {
	if !IsInteractive(in, out) || plain {
		return -1, ErrPromptCancelled
	}
	model := newChoiceModel(title, options, selected)
	program := tea.NewProgram(model, tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen())
	final, err := program.Run()
	if err != nil {
		if errors.Is(err, tea.ErrInterrupted) {
			return -1, ErrPromptCancelled
		}
		return -1, err
	}
	result, ok := final.(choiceModel)
	if !ok {
		return -1, fmt.Errorf("unexpected prompt model %T", final)
	}
	if result.cancelled {
		return -1, ErrPromptCancelled
	}
	return result.selected, nil
}

func PromptContinueOrExit(in io.Reader, out io.Writer, prompt string) (PromptAction, error) {
	model := newActionModel("Continue", prompt, []promptActionButton{
		{label: "Continue", action: PromptActionContinue},
		{label: "Exit", action: PromptActionExit},
	}, 0)
	return runActionModel(in, out, model)
}

func PromptConfirmNavigate(in io.Reader, out io.Writer, prompt string, defaultValue bool, canBack bool) (PromptAction, error) {
	buttons := []promptActionButton{
		{label: "Exit", action: PromptActionExit},
		{label: "No", action: PromptActionNo},
		{label: "Yes", action: PromptActionYes},
	}
	selected := 1
	if canBack {
		buttons = append([]promptActionButton{{label: "Back", action: PromptActionBack}}, buttons...)
		selected = 2
	}
	if defaultValue {
		selected = len(buttons) - 1
	}
	model := newActionModel("Confirm", prompt, buttons, selected)
	return runActionModel(in, out, model)
}

func runActionModel(in io.Reader, out io.Writer, model actionModel) (PromptAction, error) {
	program := tea.NewProgram(model, tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen())
	final, err := program.Run()
	if err != nil {
		if errors.Is(err, tea.ErrInterrupted) {
			return PromptActionExit, ErrPromptCancelled
		}
		return PromptActionExit, err
	}
	result, ok := final.(actionModel)
	if !ok {
		return PromptActionExit, fmt.Errorf("unexpected prompt model %T", final)
	}
	return result.selectedAction(), nil
}

func WithSpinner(ctx context.Context, in io.Reader, out io.Writer, message string, run func(context.Context) error) error {
	return WithSpinnerDone(ctx, in, out, message, message+" done", run)
}

func WithSpinnerDone(ctx context.Context, in io.Reader, out io.Writer, message string, successMessage string, run func(context.Context) error) error {
	if out == nil {
		return run(ctx)
	}
	if plain || !IsInteractive(in, out) {
		_, _ = fmt.Fprintf(out, "%s ...\n", message)
		err := run(ctx)
		if err != nil {
			_, _ = fmt.Fprintf(out, "%s failed\n", message)
			return err
		}
		_, _ = fmt.Fprintln(out, successMessage)
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- run(ctx)
	}()

	model := newSpinnerModel(message, successMessage, done, cancel)
	program := tea.NewProgram(model, tea.WithInput(in), tea.WithOutput(out), tea.WithContext(ctx))
	final, err := program.Run()
	cancel()
	if err != nil {
		if errors.Is(err, tea.ErrInterrupted) {
			return ctx.Err()
		}
		return err
	}
	result, ok := final.(spinnerModel)
	if !ok {
		return fmt.Errorf("unexpected spinner model %T", final)
	}
	return result.err
}

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

func confirmButton(label string, active bool) string {
	if active {
		return render(accent, "["+label+"]")
	}
	return render(helpText, "["+label+"]")
}

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

type spinnerTickMsg struct{}

type spinnerDoneMsg struct {
	err error
}

type spinnerModel struct {
	message        string
	successMessage string
	frames         []string
	frame          int
	done           <-chan error
	cancel         context.CancelFunc
	err            error
	finished       bool
}

func newSpinnerModel(message string, successMessage string, done <-chan error, cancel context.CancelFunc) spinnerModel {
	return spinnerModel{
		message:        message,
		successMessage: successMessage,
		frames:         []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		done:           done,
		cancel:         cancel,
	}
}

func (m spinnerModel) Init() tea.Cmd {
	return tea.Batch(spinnerTickCmd(), waitForSpinnerResult(m.done))
}

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinnerTickMsg:
		if m.finished {
			return m, nil
		}
		m.frame = (m.frame + 1) % len(m.frames)
		return m, spinnerTickCmd()
	case spinnerDoneMsg:
		m.finished = true
		m.err = msg.err
		return m, tea.Quit
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			if m.cancel != nil {
				m.cancel()
			}
			m.finished = true
			m.err = context.Canceled
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m spinnerModel) View() string {
	if m.finished {
		if m.err != nil {
			return "\n  " + render(failure, "×") + " " + m.message + " failed\n"
		}
		return "\n  " + render(success, "✓") + " " + m.successMessage + "\n"
	}
	return "\n  " + render(arrow, m.frames[m.frame]) + " " + m.message + "\n"
}

func spinnerTickCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

func waitForSpinnerResult(done <-chan error) tea.Cmd {
	return func() tea.Msg {
		return spinnerDoneMsg{err: <-done}
	}
}

func promptConfirmText(in io.Reader, out io.Writer, prompt string) (bool, error) {
	_, _ = fmt.Fprint(out, prompt)
	reader := bufio.NewReader(in)
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	if plain {
		// Keep scripts readable without cursor control.
	} else {
		_, _ = fmt.Fprint(out, "\033[1A\r\033[2K")
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}
