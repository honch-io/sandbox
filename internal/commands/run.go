package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/adapter"
	"github.com/honch/sdk/tools/sandbox/internal/config"
	"github.com/honch/sdk/tools/sandbox/internal/proxy"
	"github.com/honch/sdk/tools/sandbox/internal/runner"
	"github.com/honch/sdk/tools/sandbox/internal/session"
	"github.com/honch/sdk/tools/sandbox/internal/ui"
	"github.com/spf13/cobra"
)

func newRunCommand(deps Dependencies) *cobra.Command {
	var detach bool
	cmd := &cobra.Command{
		Use:   "run <adapter> [--detach]",
		Short: "Build and run an SDK sandbox harness",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf(ui.FormatError("missing adapter name", []ui.Row{
					{Key: "required", Value: "honch sandbox run <adapter>"},
					{Key: "example", Value: "honch sandbox run c-core --detach"},
					{Key: "adapters", Value: "honch sandbox adapters list"},
				}))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			adapterName := args[0]
			root, cfg, manager, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			if state, err := requireLiveSandbox(cmd, cfg, manager); err != nil {
				return err
			} else if runnerActive(state.Runner) {
				return fmt.Errorf(ui.FormatError("sandbox runner is already active", []ui.Row{
					{Key: "runner", Value: state.Runner.Adapter},
					{Key: "next", Value: "stop the active sandbox before starting another runner"},
					{Key: "command", Value: "honch sandbox stop"},
				}))
			}
			registry, err := adapter.LoadRegistry(root)
			if err != nil {
				return err
			}
			adapterConfig, ok := registry.Get(adapterName)
			if !ok {
				return fmt.Errorf("unsupported adapter %q; expected %s", adapterName, registry.SupportedList())
			}
			runAdapter, ok := adapterRunnerForKind(adapterConfig.Kind)
			if !ok {
				return fmt.Errorf("unsupported adapter kind %q for %s", adapterConfig.Kind, adapterConfig.Name)
			}
			return runAdapter(cmd, root, cfg, manager, adapterConfig, detach)
		},
	}
	cmd.Flags().BoolVar(&detach, "detach", false, "run harness in the background")
	return cmd
}

type adapterRunFunc func(*cobra.Command, string, config.Config, session.Manager, adapter.Config, bool) error

func adapterRunnerForKind(kind string) (adapterRunFunc, bool) {
	runners := map[string]adapterRunFunc{
		"posix":      runCCoreAdapter,
		"qemu-esp32": runEspIDFAdapter,
	}
	runAdapter, ok := runners[kind]
	return runAdapter, ok
}

func runnerActive(state session.RunnerState) bool {
	if state.Adapter == "" {
		return false
	}
	if state.PID <= 0 {
		return true
	}
	return processAlive(state.PID)
}

func requireLiveSandbox(cmd *cobra.Command, cfg config.Config, manager session.Manager) (session.State, error) {
	state, err := manager.Load()
	if err != nil {
		return session.State{}, errors.New(ui.FormatError("sandbox is not running", []ui.Row{
			{Key: "start", Value: "honch sandbox start"},
			{Key: "status", Value: "honch sandbox status"},
			{Key: "network", Value: "honch sandbox network --online"},
		}))
	}
	if !state.Stack.Running || state.Proxy.Mode != proxy.ModeOnline.String() {
		return session.State{}, errors.New(ui.FormatError("sandbox is not running", []ui.Row{
			{Key: "start", Value: "honch sandbox start"},
			{Key: "status", Value: "honch sandbox status"},
			{Key: "network", Value: "honch sandbox network --online"},
		}))
	}
	if !portIsOpen(cmd.Context(), cfg.Ports.Proxy, 200*time.Millisecond) {
		return session.State{}, errors.New(ui.FormatError("sandbox proxy is not reachable", []ui.Row{
			{Key: "proxy", Value: fmt.Sprintf("127.0.0.1:%d", cfg.Ports.Proxy)},
			{Key: "status", Value: "honch sandbox status"},
			{Key: "network", Value: "honch sandbox network --online"},
		}))
	}
	return state, nil
}

func runCCoreAdapter(cmd *cobra.Command, root string, cfg config.Config, manager session.Manager, adapterConfig adapter.Config, detach bool) error {
	r := runner.CCoreRunner{RepoRoot: root, StateDir: filepath.Join(root, cfg.Sandbox.StateDir), HarnessDir: adapterConfig.Harness}
	controlPath, err := ensureControlFIFO(root, cfg, adapterConfig.Name)
	if err != nil {
		return err
	}
	var binary string
	if err := ui.WithSpinnerDone(cmd.Context(), cmd.ErrOrStderr(), "building "+adapterConfig.Name+" harness", adapterConfig.Name+" harness has been built", func() error {
		var buildErr error
		binary, buildErr = r.Build(cmd.Context())
		return buildErr
	}); err != nil {
		return err
	}
	env := runnerEnv(cfg.Ports.Proxy, cfg.Sandbox.Token, controlPath)
	if detach {
		_, err := startRunnerSupervisor(root, cfg, adapterConfig.Name, binary, controlPath, env, func(proc *os.Process) error {
			state, _ := manager.Load()
			state.Runner = session.RunnerState{Adapter: adapterConfig.Name, PID: proc.Pid, Detached: true, ControlPath: controlPath}
			state.Proxy = runnerProxyState(cmd.Context(), cfg)
			return manager.Save(state)
		})
		if err != nil {
			return err
		}
		return nil
	}
	proc, err := runner.Start(cmd.Context(), binary, env, os.Stdin, cmd.OutOrStdout(), cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	state, _ := manager.Load()
	state.Runner = session.RunnerState{Adapter: adapterConfig.Name, PID: proc.Process.Pid, Detached: false, ControlPath: controlPath}
	state.Proxy = runnerProxyState(cmd.Context(), cfg)
	if err := saveForegroundRunnerState(manager, state, proc); err != nil {
		return err
	}
	err = proc.Wait()
	if clearErr := clearForegroundRunnerState(manager); clearErr != nil {
		return errors.Join(err, clearErr)
	}
	return err
}

func runEspIDFAdapter(cmd *cobra.Command, root string, cfg config.Config, manager session.Manager, adapterConfig adapter.Config, detach bool) error {
	idfPath, _ := resolveIDFPath(root, cfg)
	if status := qemuToolStatus(root, cfg); !status.Ready() {
		return qemuNotReadyError()
	}
	r := runner.EspIDFRunner{
		RepoRoot:        root,
		StateDir:        filepath.Join(root, cfg.Sandbox.StateDir),
		HarnessDir:      adapterConfig.Harness,
		IDFPath:         idfPath,
		Target:          adapterConfig.Build.Target,
		RunTool:         adapterConfig.Run.Tool,
		EmulatorMachine: adapterConfig.Emulator.Machine,
		EmulatorNetwork: adapterConfig.Emulator.Network,
	}
	controlPath, err := ensureControlFIFO(root, cfg, adapterConfig.Name)
	if err != nil {
		return err
	}
	var build runner.EspIDFBuild
	if err := ui.WithSpinnerDone(cmd.Context(), cmd.ErrOrStderr(), "building ESP-IDF firmware", "ESP-IDF firmware has been built", func() error {
		var buildErr error
		build, buildErr = r.Build(cmd.Context(), runner.EspIDFSettings{
			Endpoint: espIDFEndpoint(cfg),
			Token:    cfg.Sandbox.Token,
		})
		return buildErr
	}); err != nil {
		return err
	}
	if detach {
		_, err := startRunnerSupervisor(root, cfg, adapterConfig.Name, build.BuildDir, controlPath, nil, func(proc *os.Process) error {
			state, _ := manager.Load()
			state.Runner = session.RunnerState{Adapter: adapterConfig.Name, PID: proc.Pid, Detached: true, ControlPath: controlPath}
			state.Proxy = runnerProxyState(cmd.Context(), cfg)
			return manager.Save(state)
		})
		if err != nil {
			return err
		}
		return nil
	}
	state, _ := manager.Load()
	state.Runner = session.RunnerState{Adapter: adapterConfig.Name, PID: os.Getpid(), Detached: false, ControlPath: controlPath}
	state.Proxy = runnerProxyState(cmd.Context(), cfg)
	if err := manager.Save(state); err != nil {
		return err
	}
	err = r.Run(cmd.Context(), build, controlPath, cmd.OutOrStdout(), cmd.ErrOrStderr())
	if clearErr := clearForegroundRunnerState(manager); clearErr != nil {
		return errors.Join(err, clearErr)
	}
	return err
}

func saveForegroundRunnerState(manager session.Manager, state session.State, cmd *exec.Cmd) error {
	if err := manager.Save(state); err != nil {
		if cmd != nil && cmd.Process != nil {
			_ = killProcess(cmd.Process.Pid)
			_ = cmd.Wait()
		}
		return err
	}
	return nil
}

func clearForegroundRunnerState(manager session.Manager) error {
	state, err := manager.Load()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	state.Runner = session.RunnerState{}
	return manager.Save(state)
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
		Use:    "runner-serve <adapter> <target> <control-path>",
		Short:  "Run a supervised sandbox harness",
		Hidden: true,
		Args:   cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, _, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			adapterConfig, serve, err := runnerSupervisorForAdapter(root, args[0])
			if err != nil {
				return err
			}
			return serve(cmd, root, cfg, adapterConfig, args[1], args[2])
		},
	}
}

type adapterServeFunc func(*cobra.Command, string, config.Config, adapter.Config, string, string) error

func runnerSupervisorForAdapter(root string, adapterName string) (adapter.Config, adapterServeFunc, error) {
	registry, err := adapter.LoadRegistry(root)
	if err != nil {
		return adapter.Config{}, nil, err
	}
	adapterConfig, ok := registry.Get(adapterName)
	if !ok {
		return adapter.Config{}, nil, fmt.Errorf("unsupported adapter %q; expected %s", adapterName, registry.SupportedList())
	}
	serve, ok := adapterSupervisorForKind(adapterConfig.Kind)
	if !ok {
		return adapter.Config{}, nil, fmt.Errorf("unsupported adapter kind %q for %s", adapterConfig.Kind, adapterConfig.Name)
	}
	return adapterConfig, serve, nil
}

func adapterSupervisorForKind(kind string) (adapterServeFunc, bool) {
	servers := map[string]adapterServeFunc{
		"posix":      servePosixRunner,
		"qemu-esp32": serveEspIDFRunner,
	}
	serve, ok := servers[kind]
	return serve, ok
}

func servePosixRunner(cmd *cobra.Command, root string, cfg config.Config, adapterConfig adapter.Config, target string, controlPath string) error {
	proc, err := runner.Start(context.Background(), target, runnerEnv(cfg.Ports.Proxy, cfg.Sandbox.Token, controlPath), os.Stdin, cmd.OutOrStdout(), cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	return proc.Wait()
}

func serveEspIDFRunner(cmd *cobra.Command, root string, cfg config.Config, adapterConfig adapter.Config, target string, controlPath string) error {
	idfPath, _ := resolveIDFPath(root, cfg)
	r := runner.EspIDFRunner{
		RepoRoot:        root,
		StateDir:        filepath.Join(root, cfg.Sandbox.StateDir),
		HarnessDir:      adapterConfig.Harness,
		IDFPath:         idfPath,
		Target:          adapterConfig.Build.Target,
		RunTool:         adapterConfig.Run.Tool,
		EmulatorMachine: adapterConfig.Emulator.Machine,
		EmulatorNetwork: adapterConfig.Emulator.Network,
	}
	build := runner.EspIDFBuild{
		ProjectDir: filepath.Join(root, "tools", "sandbox", adapterConfig.Harness),
		BuildDir:   target,
	}
	return r.Run(context.Background(), build, controlPath, cmd.OutOrStdout(), cmd.ErrOrStderr())
}

func runnerEnv(proxyPort int, token string, controlPath string) map[string]string {
	return map[string]string{
		"HONCH_SANDBOX_ENDPOINT": fmt.Sprintf("http://127.0.0.1:%d", proxyPort),
		"HONCH_SANDBOX_TOKEN":    token,
		"HONCH_SANDBOX_CONTROL":  controlPath,
	}
}
