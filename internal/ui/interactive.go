package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

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
