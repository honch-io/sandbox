package ui

import (
	"context"
	"fmt"
	"io"
	"time"
)

func WithSpinner(ctx context.Context, out io.Writer, message string, run func() error) error {
	return WithSpinnerDone(ctx, out, message, message+" done", run)
}

func WithSpinnerDone(ctx context.Context, out io.Writer, message string, successMessage string, run func() error) error {
	if out == nil {
		return run()
	}
	if plain {
		_, _ = fmt.Fprintf(out, "%s ...\n", message)
		err := run()
		if err != nil {
			_, _ = fmt.Fprintf(out, "%s failed\n", message)
			return err
		}
		_, _ = fmt.Fprintln(out, successMessage)
		return nil
	}

	done := make(chan error, 1)
	go func() {
		done <- run()
	}()
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()
	i := 0
	for {
		select {
		case <-ctx.Done():
			_, _ = fmt.Fprintf(out, "\r%s %s cancelled\n", render(failure, "×"), message)
			return ctx.Err()
		case err := <-done:
			if err != nil {
				_, _ = fmt.Fprintf(out, "\r%s %s failed\n", render(failure, "×"), message)
				return err
			}
			_, _ = fmt.Fprintf(out, "\r%s %s\n", render(success, "✓"), successMessage)
			return nil
		case <-ticker.C:
			_, _ = fmt.Fprintf(out, "\r%s %s", render(arrow, frames[i%len(frames)]), message)
			i++
		}
	}
}
