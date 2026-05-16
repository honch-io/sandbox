package commands

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/config"
	"github.com/honch/sdk/tools/sandbox/internal/events"
	"github.com/spf13/cobra"
)

type eventTailClient interface {
	Tail(context.Context, config.Config, time.Time) (string, error)
}

const eventTailLookback = 30 * time.Second

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
			_, cfg, _, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			return tailEvents(cmd.Context(), cmd.OutOrStdout(), cfg, events.Client{}, time.Now().Add(-eventTailLookback), 2*time.Second)
		},
	})
	return cmd
}

func tailEvents(ctx context.Context, out io.Writer, cfg config.Config, client eventTailClient, since time.Time, interval time.Duration) error {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	nextSince := since
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
			_, _ = fmt.Fprint(out, result)
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
