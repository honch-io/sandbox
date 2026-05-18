package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

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
