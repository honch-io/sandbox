package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/honch/sdk/tools/sandbox/internal/config"
	"github.com/honch/sdk/tools/sandbox/internal/events"
	"github.com/honch/sdk/tools/sandbox/internal/runner"
	"github.com/honch/sdk/tools/sandbox/internal/ui"
)

var sendHarnessControlFn = sendHarnessControl
var tailEventsFn = tailEvents

func runAttachedProcessViewer(
	ctx context.Context,
	in io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	cfg config.Config,
	eventsSince time.Time,
	controlPath string,
	title string,
	start func(context.Context, io.Writer, io.Writer) error,
) error {
	if !ui.IsInteractive(in, stdout) || ui.IsPlain() {
		return start(ctx, stdout, stderr)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan error, 1)
	model := ui.NewProcessViewerModel(title, "waiting for harness output...\n", ui.ViewerFooter(), done, cancel, newProcessCommandExecutor(controlPath))
	if history, err := (events.Client{}).Tail(ctx, cfg, eventsSince); err == nil && history != "" {
		model = model.WithEventsContent(history)
	}
	program := tea.NewProgram(model, tea.WithInput(in), tea.WithOutput(stdout), tea.WithAltScreen(), tea.WithContext(ctx))
	stream := newProgramOutputStream(func(text string) {
		program.Send(ui.AppendMsg{Text: text})
	})
	eventStream := newProgramOutputStream(func(text string) {
		program.Send(ui.EventAppendMsg{Text: text})
	})

	go func() {
		err := start(ctx, stream, stream)
		stream.Flush()
		done <- err
	}()
	go func() {
		if err := tailLiveEvents(ctx, cfg, eventStream, eventsSince); err != nil && ctx.Err() == nil {
			program.Send(ui.EventAppendMsg{Text: fmt.Sprintf("\n  tail error: %v\n", err)})
		}
		eventStream.Flush()
	}()

	final, runErr := program.Run()
	cancel()
	if runErr != nil {
		if errors.Is(runErr, tea.ErrInterrupted) || errors.Is(runErr, context.Canceled) {
			return nil
		}
		return runErr
	}
	result, ok := final.(ui.ProcessViewerModel)
	if !ok {
		return fmt.Errorf("unexpected process viewer model %T", final)
	}
	if errors.Is(result.Err, context.Canceled) {
		return nil
	}
	return result.Err
}

func tailLiveEvents(ctx context.Context, cfg config.Config, out io.Writer, since time.Time) error {
	return tailEventsFn(ctx, strings.NewReader(""), out, cfg, events.Client{}, since, 2*time.Second)
}

func newProcessCommandExecutor(controlPath string) ui.ProcessCommandFunc {
	return func(raw string) (string, error) {
		action, fields, status, err := ui.ParseProcessCommand(raw)
		if err != nil {
			return "", err
		}
		if err := sendHarnessControlFn(controlPath, action, fields); err != nil {
			return "", err
		}
		if action == "track" {
			if err := sendHarnessControlFn(controlPath, "flush", nil); err != nil {
				return status, err
			}
		}
		return status, nil
	}
}

func sendHarnessControl(controlPath string, action string, fields map[string]any) error {
	if controlPath == "" {
		return fmt.Errorf("harness control is not available")
	}
	f, err := os.OpenFile(controlPath, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	return runner.SendControl(f, action, fields)
}

type programOutputStream struct {
	mu     sync.Mutex
	send   func(string)
	buffer strings.Builder
}

func newProgramOutputStream(send func(string)) *programOutputStream {
	return &programOutputStream{send: send}
}

func (w *programOutputStream) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	_, _ = w.buffer.Write(p)
	w.flushCompleteLinesLocked()
	return len(p), nil
}

func (w *programOutputStream) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.buffer.Len() == 0 {
		return
	}
	w.send(w.buffer.String())
	w.buffer.Reset()
}

func (w *programOutputStream) flushCompleteLinesLocked() {
	for {
		data := w.buffer.String()
		idx := strings.IndexByte(data, '\n')
		if idx < 0 {
			return
		}
		line := data[:idx+1]
		rest := data[idx+1:]
		w.send(line)
		w.buffer.Reset()
		_, _ = w.buffer.WriteString(rest)
	}
}
