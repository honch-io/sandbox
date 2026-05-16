package commands

import (
	"fmt"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/events"
	"github.com/spf13/cobra"
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
			_, cfg, _, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			out, err := (events.Client{}).Tail(cmd.Context(), cfg, time.Now().Add(-30*time.Second))
			if err != nil {
				return err
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), out)
			return nil
		},
	})
	return cmd
}
