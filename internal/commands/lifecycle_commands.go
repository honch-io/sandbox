package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/config"
	"github.com/honch/sdk/tools/sandbox/internal/health"
	"github.com/honch/sdk/tools/sandbox/internal/proxy"
	"github.com/honch/sdk/tools/sandbox/internal/session"
	"github.com/honch/sdk/tools/sandbox/internal/stack"
	"github.com/honch/sdk/tools/sandbox/internal/ui"
	"github.com/spf13/cobra"
)

func newStartCommand(deps Dependencies) *cobra.Command {
	var migrate bool
	var skipMigrations bool
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the real local Honch stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			if migrate && skipMigrations {
				return errors.New(ui.FormatError("choose one migration mode", []ui.Row{
					{Key: "--migrate", Value: "run platform migrations without prompting"},
					{Key: "--skip-migrations", Value: "start without running platform migrations"},
				}))
			}
			root, cfg, manager, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			existingState, _ := manager.Load()
			if existingState.Stack.Running {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), ui.Success("sandbox is already running"))
				return nil
			}
			if err := writeProxyMode(root, cfg, proxy.ModeOnline); err != nil {
				return err
			}
			service := stack.New(root)
			if cfg.Repos.Platform != "" {
				switch {
				case migrate:
					service.ApproveMigrations = func() (bool, error) {
						return true, nil
					}
				case skipMigrations:
					service.SkipMigrations = true
				default:
					if ui.IsInteractive(cmd.InOrStdin(), cmd.OutOrStdout()) && !ui.IsPlain() {
						choice, err := ui.PromptChoice(cmd.InOrStdin(), cmd.OutOrStdout(), "Platform database migrations", []ui.PromptOption{
							{Label: "Run migrations", Description: "run platform migrations before starting"},
							{Label: "Skip migrations", Description: "start without running platform migrations"},
							{Label: "Cancel", Description: "abort the start command"},
						}, 1)
						if err != nil {
							if errors.Is(err, ui.ErrPromptCancelled) {
								return fmt.Errorf("start cancelled")
							}
							return err
						}
						switch choice {
						case 0:
							service.ApproveMigrations = func() (bool, error) {
								return true, nil
							}
						case 1:
							service.SkipMigrations = true
						default:
							return fmt.Errorf("start cancelled")
						}
					} else {
						approved, err := confirm(cmd.InOrStdin(), cmd.OutOrStdout(), "Run platform database migrations with `bun run db:migrate`? [y/N] ")
						if err != nil {
							return err
						}
						if approved {
							service.ApproveMigrations = func() (bool, error) {
								return true, nil
							}
						} else {
							service.SkipMigrations = true
						}
					}
				}
			}
			if ui.IsInteractive(cmd.InOrStdin(), cmd.ErrOrStderr()) && !ui.IsPlain() {
				if err := resolveProxyPortConflict(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr(), root, cfg); err != nil {
					return err
				}
			}
			if err := ui.WithSpinnerDone(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr(), "starting sandbox", "sandbox has been started", func(ctx context.Context) error {
				if err := service.Start(ctx, cfg); err != nil {
					rollbackStartedSandbox(cmd.Context(), root, cfg, manager, nil)
					return err
				}
				proxyProc, err := startProxyProcess(ctx, root, cfg, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
				if err != nil {
					rollbackStartedSandbox(cmd.Context(), root, cfg, manager, nil)
					return err
				}
				state := session.State{
					StartedAt: time.Now().UTC(),
					Stack:     session.StackState{Running: true},
					Runner:    existingState.Runner,
					Proxy:     session.ProxyState{Mode: proxy.ModeOnline.String(), Port: cfg.Ports.Proxy, PID: proxySessionPID(root, cfg, existingState.Proxy, proxyProc)},
				}
				if err := manager.Save(state); err != nil {
					rollbackStartedSandbox(cmd.Context(), root, cfg, manager, proxyProc)
					return err
				}
				return nil
			}); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&migrate, "migrate", false, "run platform migrations without prompting")
	cmd.Flags().BoolVar(&skipMigrations, "skip-migrations", false, "start without running platform migrations")
	return cmd
}

func newStopCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the active sandbox session",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, manager, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			state, err := manager.Load()
			if err != nil || !sandboxHasActiveProcesses(state) {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.Success("sandbox is not running"))
				return nil
			}
			if state.Proxy.PID > 0 {
				_ = killProcess(state.Proxy.PID)
			}
			_ = os.Remove(proxyPIDPath(root, cfg))
			if state.Runner.PID > 0 && state.Runner.Detached {
				_ = killProcess(state.Runner.PID)
			}
			_ = killSandboxRunnerProcesses(root, cfg)
			if state.Stack.Running {
				if err := stack.New(root).Stop(cmd.Context(), cfg); err != nil {
					return err
				}
			}
			if err := manager.Clear(); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.Success("sandbox has been stopped"))
			return nil
		},
	}
}

func proxySessionPID(root string, cfg config.Config, existing session.ProxyState, proc *os.Process) int {
	if proc != nil {
		return proc.Pid
	}
	if existing.PID > 0 {
		return existing.PID
	}
	if pid, ok := readPID(proxyPIDPath(root, cfg)); ok {
		return pid
	}
	return 0
}

func rollbackStartedSandbox(ctx context.Context, root string, cfg config.Config, manager session.Manager, proxyProc *os.Process) {
	if proxyProc != nil {
		_ = killProcess(proxyProc.Pid)
	}
	_ = os.Remove(proxyPIDPath(root, cfg))
	_ = stack.New(root).Stop(ctx, cfg)
	_ = manager.Clear()
}

func sandboxHasActiveProcesses(state session.State) bool {
	return state.Stack.Running || state.Runner.Adapter != "" || state.Runner.PID > 0 || state.Proxy.PID > 0
}

func newStatusCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show stack, runner, proxy, and repo status",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, manager, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			sessionRows := []ui.Row{}
			if state, err := manager.Load(); err == nil {
				sessionRows = append(sessionRows,
					ui.Row{Key: "session", Value: state.ID},
					ui.Row{Key: "runner", Value: valueOr(state.Runner.Adapter, "none")},
					ui.Row{Key: "proxy", Value: valueOr(state.Proxy.Mode, "online")},
				)
			} else {
				sessionRows = append(sessionRows, ui.Row{Key: "session", Value: "inactive"})
			}
			repoRows := []ui.Row{}
			repoHealth := stack.New(root).Health(cmd.Context(), cfg)
			for _, name := range []string{"worker", "capture", "platform"} {
				if state, ok := repoHealth[name]; ok {
					repoRows = append(repoRows, ui.Row{Key: name, Value: state})
				}
			}
			portRows := []ui.Row{
				{Key: "capture port", Value: cfg.Ports.Capture},
				{Key: "worker port", Value: cfg.Ports.Worker},
				{Key: "clickhouse port", Value: cfg.Ports.ClickHouse},
				{Key: "proxy port", Value: cfg.Ports.Proxy},
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.FormatSections("Honch sandbox", []ui.Section{
				{Name: "session", Rows: sessionRows},
				{Name: "repos", Rows: repoRows},
				{Name: "services", Rows: serviceHealthRows(cmd.Context(), cfg)},
				{Name: "ports", Rows: portRows},
			}))
			return nil
		},
	}
}

func serviceHealthRows(ctx context.Context, cfg config.Config) []ui.Row {
	checkTimeout := 750 * time.Millisecond
	return []ui.Row{
		{Key: "postgres", Value: health.TCPStatus(ctx, "127.0.0.1:5432", checkTimeout)},
		{Key: "redis", Value: health.TCPStatus(ctx, "127.0.0.1:6379", checkTimeout)},
		{Key: "pubsub", Value: health.TCPStatus(ctx, "127.0.0.1:8085", checkTimeout)},
		{Key: "clickhouse", Value: health.ClickHouseStatus(ctx, fmt.Sprintf("127.0.0.1:%d", cfg.Ports.ClickHouse), checkTimeout)},
		{Key: "capture health", Value: health.HTTPStatus(ctx, fmt.Sprintf("http://127.0.0.1:%d/health", cfg.Ports.Capture), checkTimeout)},
		{Key: "worker health", Value: health.HTTPStatus(ctx, fmt.Sprintf("http://127.0.0.1:%d/", cfg.Ports.Worker), checkTimeout)},
		{Key: "proxy health", Value: health.TCPStatus(ctx, fmt.Sprintf("127.0.0.1:%d", cfg.Ports.Proxy), checkTimeout)},
	}
}

func newUpdateCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Fetch and fast-forward clean sibling stack repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, _, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			if err := stack.New(root).Update(cmd.Context(), cfg); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.Success("stack repos have been updated"))
			return nil
		},
	}
}
