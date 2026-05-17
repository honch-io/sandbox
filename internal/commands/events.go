package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/honch/sdk/tools/sandbox/internal/config"
	"github.com/honch/sdk/tools/sandbox/internal/events"
	"github.com/honch/sdk/tools/sandbox/internal/session"
	"github.com/honch/sdk/tools/sandbox/internal/ui"
	"github.com/spf13/cobra"
)

type eventTailClient interface {
	Tail(context.Context, config.Config, time.Time) (string, error)
}

const (
	eventTailLookback  = 30 * time.Second
	eventTailSeenLimit = 1000
)

func newEventsCommand(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{Use: "events", Short: "Query ClickHouse sandbox events", Args: rejectUnknownArgs, RunE: commandGroupRunE}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List recent ingested events",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, _, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			out, err := (events.Client{}).List(cmd.Context(), cfg, 25)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), out)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "tail",
		Short: "Poll ClickHouse for newly ingested events",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, manager, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			return tailEvents(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), cfg, events.Client{}, eventTailSince(manager), 2*time.Second)
		},
	})
	return cmd
}

func eventTailSince(manager session.Manager) time.Time {
	state, err := manager.Load()
	if err == nil && !state.StartedAt.IsZero() {
		return state.StartedAt
	}
	return time.Now().Add(-eventTailLookback)
}

func tailEvents(ctx context.Context, in io.Reader, out io.Writer, cfg config.Config, client eventTailClient, since time.Time, interval time.Duration) error {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	if ui.IsInteractive(in, out) && !ui.IsPlain() {
		return tailEventsInteractive(ctx, in, out, cfg, client, since, interval)
	}
	nextSince := since
	seen := newTailSeen(eventTailSeenLimit)
	for {
		pollStarted := time.Now().UTC()
		result, err := client.Tail(ctx, cfg, nextSince)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if result != "" {
			if _, err := writeUnseenTailRows(out, result, seen); err != nil {
				return err
			}
		}
		nextSince = pollStarted.Add(-eventTailLookback)
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func tailEventsInteractive(ctx context.Context, in io.Reader, out io.Writer, cfg config.Config, client eventTailClient, since time.Time, interval time.Duration) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	model := ui.NewTextViewerModel("Honch sandbox events tail", "waiting for events...\n", ui.ViewerFooter())
	program := tea.NewProgram(model, tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen(), tea.WithContext(ctx))
	errCh := make(chan error, 1)
	go func() {
		nextSince := since
		seen := newTailSeen(eventTailSeenLimit)
		for {
			pollStarted := time.Now().UTC()
			result, err := client.Tail(ctx, cfg, nextSince)
			if err != nil {
				if ctx.Err() == nil {
					errCh <- err
					cancel()
				}
				return
			}
			if result != "" {
				var b strings.Builder
				if _, writeErr := writeUnseenTailRows(&b, result, seen); writeErr != nil {
					errCh <- writeErr
					cancel()
					return
				}
				if b.Len() > 0 {
					program.Send(ui.AppendMsg{Text: b.String()})
				}
			}
			nextSince = pollStarted.Add(-eventTailLookback)
			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	_, runErr := program.Run()
	cancel()
	if runErr != nil {
		if errors.Is(runErr, tea.ErrInterrupted) {
			return nil
		}
		return runErr
	}
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

func writeUnseenTailRows(out io.Writer, rows string, seen *tailSeen) (int, error) {
	written := 0
	for rows != "" {
		line := rows
		if idx := strings.IndexByte(rows, '\n'); idx >= 0 {
			line = rows[:idx+1]
			rows = rows[idx+1:]
		} else {
			rows = ""
		}
		key := strings.TrimRight(line, "\r\n")
		if key == "" {
			continue
		}
		if !seen.remember(key) {
			continue
		}
		n, err := fmt.Fprint(out, line)
		written += n
		if err != nil {
			return written, err
		}
	}
	return written, nil
}

type tailSeen struct {
	limit int
	keys  map[string]struct{}
	order []string
}

func newTailSeen(limit int) *tailSeen {
	if limit <= 0 {
		limit = eventTailSeenLimit
	}
	return &tailSeen{limit: limit, keys: map[string]struct{}{}, order: make([]string, 0, limit)}
}

func (s *tailSeen) remember(key string) bool {
	if _, ok := s.keys[key]; ok {
		return false
	}
	s.keys[key] = struct{}{}
	s.order = append(s.order, key)
	for len(s.order) > s.limit {
		delete(s.keys, s.order[0])
		s.order = s.order[1:]
	}
	return true
}
