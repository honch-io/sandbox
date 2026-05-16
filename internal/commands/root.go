package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/adapter"
	"github.com/honch/sdk/tools/sandbox/internal/config"
	"github.com/honch/sdk/tools/sandbox/internal/health"
	"github.com/honch/sdk/tools/sandbox/internal/proxy"
	"github.com/honch/sdk/tools/sandbox/internal/runner"
	"github.com/honch/sdk/tools/sandbox/internal/session"
	"github.com/honch/sdk/tools/sandbox/internal/stack"
	"github.com/honch/sdk/tools/sandbox/internal/ui"
	"github.com/spf13/cobra"
)

type Dependencies struct {
	RootDir string
	In      io.Reader
	Out     io.Writer
	Err     io.Writer
}

func NewRootCommand(deps Dependencies) *cobra.Command {
	deps = withDefaultIO(deps)
	var plain bool
	root := &cobra.Command{
		Use:           "honch",
		Short:         "Honch developer tooling",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ui.SetPlain(plain)
		},
	}
	root.PersistentFlags().BoolVar(&plain, "plain", false, "disable styled output")
	root.SetIn(deps.In)
	root.SetOut(deps.Out)
	root.SetErr(deps.Err)
	root.AddCommand(newSandboxCommand(deps))
	installHelp(root)
	return root
}

func withDefaultIO(deps Dependencies) Dependencies {
	if deps.In == nil {
		deps.In = os.Stdin
	}
	if deps.Out == nil {
		deps.Out = os.Stdout
	}
	if deps.Err == nil {
		deps.Err = os.Stderr
	}
	return deps
}

func newSandboxCommand(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sandbox",
		Short: "Run the Honch SDK E2E sandbox",
	}
	cmd.AddCommand(
		newDoctorCommand(deps),
		newSetupCommand(deps),
		newStartCommand(deps),
		newStopCommand(deps),
		newStatusCommand(deps),
		newUpdateCommand(deps),
		newRunCommand(deps),
		newBatteryCommand(deps),
		newNetworkCommand(deps),
		newTrackCommand(deps),
		newFlushCommand(deps),
		newResetCommand(deps),
		newLogsCommand(deps),
		newEventsCommand(deps),
		newScenarioCommand(deps),
		newQEMUCommand(deps),
		newProxyServeCommand(deps),
		newRunnerServeCommand(deps),
	)
	return cmd
}

func newStartCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the real local Honch stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, manager, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			if err := writeProxyMode(root, cfg, proxy.ModeOnline); err != nil {
				return err
			}
			existingState, _ := manager.Load()
			service := stack.New(root)
			service.ApproveMigrations = func() (bool, error) {
				return confirm(cmd.InOrStdin(), cmd.OutOrStdout(), "Run platform database migrations with `bun run db:migrate`? [y/N] ")
			}
			if err := service.Start(cmd.Context(), cfg); err != nil {
				if errors.Is(err, stack.ErrMigrationDeclined) {
					return fmt.Errorf("start cancelled before migrations")
				}
				return err
			}
			proxyProc, err := startProxyProcess(root, cfg)
			if err != nil {
				return err
			}
			proxyPID := existingState.Proxy.PID
			if proxyProc != nil {
				proxyPID = proxyProc.Pid
			}
			state := session.State{
				StartedAt: time.Now().UTC(),
				Stack:     session.StackState{Running: true},
				Runner:    existingState.Runner,
				Proxy:     session.ProxyState{Mode: proxy.ModeOnline.String(), Port: cfg.Ports.Proxy, PID: proxyPID},
			}
			if err := manager.Save(state); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), ui.Heading("sandbox started"))
			return nil
		},
	}
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
			state, _ := manager.Load()
			if state.Proxy.PID > 0 {
				_ = killProcess(state.Proxy.PID)
			}
			_ = killPortListener(cfg.Ports.Proxy)
			if state.Runner.PID > 0 && state.Runner.Detached {
				_ = killProcess(state.Runner.PID)
			}
			_ = killSandboxRunnerProcesses(root, cfg)
			if err := stack.New(root).Stop(cmd.Context(), cfg); err != nil {
				return err
			}
			if err := manager.Clear(); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "sandbox stopped")
			return nil
		},
	}
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
				ui.Row{Key: "capture port", Value: cfg.Ports.Capture},
				ui.Row{Key: "worker port", Value: cfg.Ports.Worker},
				ui.Row{Key: "clickhouse port", Value: cfg.Ports.ClickHouse},
				ui.Row{Key: "proxy port", Value: cfg.Ports.Proxy},
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
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "stack repos updated")
			return nil
		},
	}
}

func newRunCommand(deps Dependencies) *cobra.Command {
	var detach bool
	cmd := &cobra.Command{
		Use:   "run <c-core|esp-idf> [--detach]",
		Short: "Build and run an SDK sandbox harness",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			adapterName := args[0]
			root, cfg, manager, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			registry, err := adapter.LoadRegistry(root)
			if err != nil {
				return err
			}
			adapterConfig, ok := registry.Get(adapterName)
			if !ok {
				return fmt.Errorf("unsupported adapter %q; expected %s", adapterName, registry.SupportedList())
			}
			switch adapterConfig.Kind {
			case "posix":
				return runCCoreAdapter(cmd, root, cfg, manager, detach)
			case "qemu-esp32":
				return runEspIDFAdapter(cmd, root, cfg, manager, detach)
			default:
				return fmt.Errorf("unsupported adapter kind %q for %s", adapterConfig.Kind, adapterConfig.Name)
			}
		},
	}
	cmd.Flags().BoolVar(&detach, "detach", false, "run harness in the background")
	return cmd
}

func runCCoreAdapter(cmd *cobra.Command, root string, cfg config.Config, manager session.Manager, detach bool) error {
	r := runner.CCoreRunner{RepoRoot: root, StateDir: filepath.Join(root, cfg.Sandbox.StateDir)}
	controlPath, err := ensureControlFIFO(root, cfg, "c-core")
	if err != nil {
		return err
	}
	binary, err := r.Build(cmd.Context())
	if err != nil {
		return err
	}
	env := runnerEnv(cfg.Ports.Proxy, cfg.Sandbox.Token, controlPath)
	if detach {
		proc, err := startRunnerSupervisor(root, cfg, "c-core", binary, controlPath, env)
		if err != nil {
			return err
		}
		state, _ := manager.Load()
		state.Runner = session.RunnerState{Adapter: "c-core", PID: proc.Pid, Detached: true, ControlPath: controlPath}
		state.Proxy = runnerProxyState(cmd.Context(), cfg)
		return manager.Save(state)
	}
	proc, err := runner.Start(cmd.Context(), binary, env, os.Stdin, cmd.OutOrStdout(), cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	state, _ := manager.Load()
	state.Runner = session.RunnerState{Adapter: "c-core", PID: proc.Process.Pid, Detached: false, ControlPath: controlPath}
	state.Proxy = runnerProxyState(cmd.Context(), cfg)
	if err := manager.Save(state); err != nil {
		return err
	}
	err = proc.Wait()
	state, _ = manager.Load()
	state.Runner = session.RunnerState{}
	_ = manager.Save(state)
	return err
}

func runEspIDFAdapter(cmd *cobra.Command, root string, cfg config.Config, manager session.Manager, detach bool) error {
	idfPath, _ := resolveIDFPath(root, cfg)
	if status := qemuToolStatus(root, cfg); !status.Ready() {
		return qemuNotReadyError()
	}
	r := runner.EspIDFRunner{RepoRoot: root, StateDir: filepath.Join(root, cfg.Sandbox.StateDir), IDFPath: idfPath}
	controlPath, err := ensureControlFIFO(root, cfg, "esp-idf")
	if err != nil {
		return err
	}
	build, err := r.Build(cmd.Context(), runner.EspIDFSettings{
		Endpoint: espIDFEndpoint(cfg),
		Token:    cfg.Sandbox.Token,
	})
	if err != nil {
		return err
	}
	if detach {
		proc, err := startRunnerSupervisor(root, cfg, "esp-idf", build.BuildDir, controlPath, nil)
		if err != nil {
			return err
		}
		state, _ := manager.Load()
		state.Runner = session.RunnerState{Adapter: "esp-idf", PID: proc.Pid, Detached: true, ControlPath: controlPath}
		state.Proxy = runnerProxyState(cmd.Context(), cfg)
		return manager.Save(state)
	}
	state, _ := manager.Load()
	state.Runner = session.RunnerState{Adapter: "esp-idf", PID: os.Getpid(), Detached: false, ControlPath: controlPath}
	state.Proxy = runnerProxyState(cmd.Context(), cfg)
	if err := manager.Save(state); err != nil {
		return err
	}
	err = r.Run(cmd.Context(), build, controlPath, cmd.OutOrStdout(), cmd.ErrOrStderr())
	state, _ = manager.Load()
	state.Runner = session.RunnerState{}
	_ = manager.Save(state)
	return err
}

func espIDFEndpoint(cfg config.Config) string {
	return fmt.Sprintf("http://10.0.2.2:%d", cfg.Ports.Proxy)
}

func runnerProxyState(ctx context.Context, cfg config.Config) session.ProxyState {
	mode := "not running"
	if portIsOpen(ctx, cfg.Ports.Proxy, 200*time.Millisecond) {
		mode = proxy.ModeOnline.String()
	}
	return session.ProxyState{Mode: mode, Port: cfg.Ports.Proxy}
}

func newRunnerServeCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:    "runner-serve <adapter> <binary> <control-path>",
		Short:  "Run a supervised sandbox harness",
		Hidden: true,
		Args:   cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, _, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			switch args[0] {
			case "c-core":
				proc, err := runner.Start(context.Background(), args[1], runnerEnv(cfg.Ports.Proxy, cfg.Sandbox.Token, args[2]), os.Stdin, cmd.OutOrStdout(), cmd.ErrOrStderr())
				if err != nil {
					return err
				}
				return proc.Wait()
			case "esp-idf":
				idfPath, _ := resolveIDFPath(root, cfg)
				r := runner.EspIDFRunner{RepoRoot: root, StateDir: filepath.Join(root, cfg.Sandbox.StateDir), IDFPath: idfPath}
				build := runner.EspIDFBuild{
					ProjectDir: filepath.Join(root, "tools", "sandbox", "harnesses", "esp-idf"),
					BuildDir:   args[1],
				}
				return r.Run(context.Background(), build, args[2], cmd.OutOrStdout(), cmd.ErrOrStderr())
			default:
				return fmt.Errorf("unsupported adapter %q; expected c-core or esp-idf", args[0])
			}
		},
	}
}

func runnerEnv(proxyPort int, token string, controlPath string) map[string]string {
	return map[string]string{
		"HONCH_SANDBOX_ENDPOINT": fmt.Sprintf("http://127.0.0.1:%d", proxyPort),
		"HONCH_SANDBOX_TOKEN":    token,
		"HONCH_SANDBOX_CONTROL":  controlPath,
	}
}

func newBatteryCommand(deps Dependencies) *cobra.Command {
	var level int
	cmd := liveControlCommand(deps, "battery", "Set the live harness battery level", func(w io.Writer) error {
		if level < 0 || level > 100 {
			return fmt.Errorf("level must be between 0 and 100")
		}
		return runner.SendControl(w, "battery", map[string]any{"level": level})
	})
	cmd.Flags().IntVar(&level, "level", -1, "battery level from 0 to 100")
	_ = cmd.MarkFlagRequired("level")
	return cmd
}

func newNetworkCommand(deps Dependencies) *cobra.Command {
	var online, offline, serverError bool
	cmd := &cobra.Command{
		Use:   "network --online|--offline|--server-error",
		Short: "Control the sandbox proxy network mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			count := boolCount(online, offline, serverError)
			if count != 1 {
				return errors.New(ui.FormatError("choose one network mode", []ui.Row{
					{Key: "required", Value: "--online, --offline, or --server-error"},
					{Key: "example", Value: "honch sandbox network --offline"},
				}))
			}
			mode := selectedNetworkMode(offline, serverError)
			root, cfg, manager, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			if err := writeProxyMode(root, cfg, mode); err != nil {
				return err
			}
			if err := saveProxyStateIfActive(manager, cfg, mode); err != nil {
				return err
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.FormatKeyValues("Network", []ui.Row{{Key: "mode", Value: mode}}))
			return nil
		},
	}
	cmd.Flags().BoolVar(&online, "online", false, "forward SDK HTTP to capture")
	cmd.Flags().BoolVar(&offline, "offline", false, "return network-unavailable errors")
	cmd.Flags().BoolVar(&serverError, "server-error", false, "return HTTP 500 responses")
	return cmd
}

func selectedNetworkMode(offline bool, serverError bool) proxy.Mode {
	if offline {
		return proxy.ModeOffline
	}
	if serverError {
		return proxy.ModeServerError
	}
	return proxy.ModeOnline
}

func saveProxyStateIfActive(manager session.Manager, cfg config.Config, mode proxy.Mode) error {
	state, err := manager.Load()
	if err != nil {
		return nil
	}
	state.Proxy.Mode = mode.String()
	state.Proxy.Port = cfg.Ports.Proxy
	return manager.Save(state)
}

func newTrackCommand(deps Dependencies) *cobra.Command {
	var properties string
	cmd := liveControlCommand(deps, "track <event>", "Ask the harness to emit a custom event", func(w io.Writer) error {
		return runner.SendControl(w, "track", map[string]any{"event": "<event>", "properties": properties})
	})
	cmd.Args = func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return errors.New(ui.FormatError("missing event name", []ui.Row{
				{Key: "required", Value: "honch sandbox track <event>"},
				{Key: "example", Value: "honch sandbox track camera.motion --properties '{\"zone\":\"porch\"}'"},
			}))
		}
		return nil
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		props, err := parseProperties(properties)
		if err != nil {
			return err
		}
		if err := writeHarnessControl(deps, func(w io.Writer) error {
			return runner.SendControl(w, "track", map[string]any{"event": args[0], "properties": props})
		}); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "sent track control: %s\n", args[0])
		return nil
	}
	cmd.Flags().StringVar(&properties, "properties", "{}", "JSON object properties")
	return cmd
}

func parseProperties(raw string) (map[string]any, error) {
	props := map[string]any{}
	if raw == "" {
		return props, nil
	}
	if !json.Valid([]byte(raw)) {
		return nil, errors.New(ui.FormatError("properties must be valid JSON", []ui.Row{
			{Key: "example", Value: `--properties '{"zone":"porch"}'`},
		}))
	}
	if err := json.Unmarshal([]byte(raw), &props); err != nil {
		return nil, errors.New(ui.FormatError("properties must be a valid JSON object", []ui.Row{
			{Key: "example", Value: `--properties '{"zone":"porch"}'`},
		}))
	}
	return props, nil
}

func newFlushCommand(deps Dependencies) *cobra.Command {
	return liveControlCommand(deps, "flush", "Ask the harness to flush queued events", func(w io.Writer) error {
		return runner.SendControl(w, "flush", nil)
	})
}

func newResetCommand(deps Dependencies) *cobra.Command {
	return liveControlCommand(deps, "reset", "Ask the harness to run SDK reset behavior", func(w io.Writer) error {
		return runner.SendControl(w, "reset", nil)
	})
}

func newProxyServeCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:    "proxy-serve",
		Short:  "Run the local sandbox HTTP proxy",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, _, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			controller := proxy.NewController(proxy.ModeOnline)
			handler, err := controller.Handler(cfg.Sandbox.EndpointURL)
			if err != nil {
				return err
			}
			modePath := proxyModePath(root, cfg)
			wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if data, err := os.ReadFile(modePath); err == nil {
					if mode, err := proxy.ParseMode(stringTrim(data)); err == nil {
						controller.SetMode(mode)
					}
				}
				handler.ServeHTTP(w, r)
			})
			server := &http.Server{
				Addr:              fmt.Sprintf("127.0.0.1:%d", cfg.Ports.Proxy),
				Handler:           wrapped,
				ReadHeaderTimeout: 5 * time.Second,
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "proxy listening on %s\n", server.Addr)
			return server.ListenAndServe()
		},
	}
}

func newLogsCommand(deps Dependencies) *cobra.Command {
	var tail int
	cmd := &cobra.Command{
		Use:   "logs [stack|device|proxy]",
		Short: "Print recent sandbox logs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, _, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			target := "stack"
			if len(args) == 1 {
				target = args[0]
			}
			return printLogs(cmd.OutOrStdout(), root, cfg, target, logOptions{Tail: tail})
		},
	}
	cmd.Flags().IntVar(&tail, "tail", 80, "number of recent log lines to print per file")
	return cmd
}
