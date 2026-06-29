package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"honch.dev/honch/internal/adapter"
	"honch.dev/honch/internal/config"
	"honch.dev/honch/internal/proxy"
	"honch.dev/honch/internal/runner"
	"honch.dev/honch/internal/session"
	"honch.dev/honch/internal/ui"
)

func newRunCommand(deps Dependencies) *cobra.Command {
	var opts runOptions
	cmd := &cobra.Command{
		Use:   "run <adapter> [--detach]",
		Short: "Build and run an SDK sandbox harness",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New(ui.FormatError("missing adapter name", []ui.Row{
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
				stopCommand := "honch sandbox stop " + state.Runner.Adapter
				if state.Runner.Adapter == "esp-idf" {
					stopCommand = "honch sandbox qemu stop"
				}
				return errors.New(ui.FormatError("sandbox runner is already active", []ui.Row{
					{Key: "runner", Value: state.Runner.Adapter},
					{Key: "next", Value: "stop the active runner before starting another one"},
					{Key: "command", Value: stopCommand},
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
			return runAdapter(cmd, root, cfg, manager, adapterConfig, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.Detach, "detach", false, "run harness in the background")
	cmd.Flags().StringVar(&opts.DevicePort, "device", "", "serial port for a real ESP-IDF device")
	cmd.Flags().StringVar(&opts.DeviceEndpoint, "device-endpoint", "", "sandbox proxy URL reachable from a real device")
	cmd.Flags().StringVar(&opts.WiFiSSID, "wifi-ssid", "", "Wi-Fi SSID for ESP-IDF hardware runs")
	cmd.Flags().StringVar(&opts.WiFiPassword, "wifi-password", "", "Wi-Fi password for ESP-IDF hardware runs")
	cmd.Flags().BoolVar(&opts.EraseFlash, "erase-flash", false, "erase the ESP-IDF device before flashing")
	return cmd
}

type runOptions struct {
	Detach         bool
	DevicePort     string
	DeviceEndpoint string
	WiFiSSID       string
	WiFiPassword   string
	EraseFlash     bool
}

type adapterRunFunc func(*cobra.Command, string, config.Config, session.Manager, adapter.Config, runOptions) error

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

func runCCoreAdapter(cmd *cobra.Command, root string, cfg config.Config, manager session.Manager, adapterConfig adapter.Config, opts runOptions) error {
	r := runner.CCoreRunner{RepoRoot: root, StateDir: filepath.Join(root, cfg.Sandbox.StateDir), HarnessDir: adapterConfig.Harness}
	controlPath, err := ensureControlFIFO(root, cfg, adapterConfig.Name)
	if err != nil {
		return err
	}
	var binary string
	if err := ui.WithSpinnerDone(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr(), "building "+adapterConfig.Name+" harness", adapterConfig.Name+" harness has been built", func(ctx context.Context) error {
		var buildErr error
		binary, buildErr = r.Build(ctx)
		return buildErr
	}); err != nil {
		return err
	}
	env := runnerEnv(cfg.Ports.Proxy, cfg.Sandbox.Token, controlPath)
	if opts.Detach {
		_, err := startRunnerSupervisor(root, cfg, adapterConfig.Name, binary, controlPath, env, func(proc *os.Process) error {
			return saveRunnerSessionState(manager, session.RunnerState{Adapter: adapterConfig.Name, PID: proc.Pid, Detached: true, ControlPath: controlPath}, runnerProxyState(cmd.Context(), cfg))
		})
		if err != nil {
			return err
		}
		return nil
	}
	return runAttachedProcessViewer(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr(), cfg, time.Now().UTC(), controlPath, "Honch sandbox run "+adapterConfig.Name, func(ctx context.Context, stdout io.Writer, stderr io.Writer) error {
		proc, err := runner.Start(ctx, binary, env, nil, stdout, stderr)
		if err != nil {
			return err
		}
		if err := saveForegroundRunnerSessionState(manager, session.RunnerState{Adapter: adapterConfig.Name, PID: proc.Process.Pid, Detached: false, ControlPath: controlPath}, runnerProxyState(ctx, cfg), proc); err != nil {
			return err
		}
		err = proc.Wait()
		if clearErr := clearForegroundRunnerState(manager); clearErr != nil {
			return errors.Join(err, clearErr)
		}
		return err
	})
}

func runEspIDFAdapter(cmd *cobra.Command, root string, cfg config.Config, manager session.Manager, adapterConfig adapter.Config, opts runOptions) error {
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
	if opts.DevicePort != "" {
		return runEspIDFHardwareAdapter(cmd, root, cfg, manager, r, adapterConfig, opts)
	}
	var build runner.EspIDFBuild
	if err := ui.WithSpinnerDone(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr(), "building ESP-IDF firmware", "ESP-IDF firmware has been built", func(ctx context.Context) error {
		var buildErr error
		build, buildErr = r.Build(ctx, runner.EspIDFSettings{
			Endpoint: espIDFEndpoint(cfg),
			Token:    cfg.Sandbox.Token,
		})
		return buildErr
	}); err != nil {
		return err
	}
	if opts.Detach {
		_, err := startRunnerSupervisor(root, cfg, adapterConfig.Name, build.BuildDir, controlPath, nil, func(proc *os.Process) error {
			return saveRunnerSessionState(manager, session.RunnerState{Adapter: adapterConfig.Name, PID: proc.Pid, Detached: true, ControlPath: controlPath}, runnerProxyState(cmd.Context(), cfg))
		})
		if err != nil {
			return err
		}
		return nil
	}
	return runAttachedProcessViewer(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr(), cfg, time.Now().UTC(), controlPath, "Honch sandbox run "+adapterConfig.Name, func(ctx context.Context, stdout io.Writer, stderr io.Writer) error {
		if err := saveRunnerSessionState(manager, session.RunnerState{Adapter: adapterConfig.Name, PID: os.Getpid(), Detached: false, ControlPath: controlPath}, runnerProxyState(ctx, cfg)); err != nil {
			return err
		}
		err := r.Run(ctx, build, controlPath, stdout, stderr)
		if clearErr := clearForegroundRunnerState(manager); clearErr != nil {
			return errors.Join(err, clearErr)
		}
		return err
	})
}

func runEspIDFHardwareAdapter(cmd *cobra.Command, root string, cfg config.Config, manager session.Manager, r runner.EspIDFRunner, adapterConfig adapter.Config, opts runOptions) error {
	if opts.Detach {
		return fmt.Errorf("ESP-IDF hardware runs must stay attached so the serial monitor can own the TTY")
	}
	if cfg.Sandbox.ProxyBind == "127.0.0.1" || cfg.Sandbox.ProxyBind == "localhost" {
		return errors.New(ui.FormatError("sandbox proxy is bound to localhost", []ui.Row{
			{Key: "required", Value: "set sandbox.proxy_bind to 0.0.0.0 and restart the stack"},
			{Key: "command", Value: "honch sandbox config set sandbox.proxy_bind 0.0.0.0"},
		}))
	}
	localSSID, localPassword, _ := sdkLocalWiFiDefaults(root)
	wifiSSID := valueOr(opts.WiFiSSID, os.Getenv("HONCH_SANDBOX_WIFI_SSID"), localSSID)
	wifiPassword := valueOr(opts.WiFiPassword, os.Getenv("HONCH_SANDBOX_WIFI_PASSWORD"), localPassword)
	if wifiSSID == "" || wifiPassword == "" {
		return errors.New(ui.FormatError("Wi-Fi credentials are required for ESP-IDF hardware runs", []ui.Row{
			{Key: "flags", Value: "--wifi-ssid and --wifi-password"},
			{Key: "env", Value: "HONCH_SANDBOX_WIFI_SSID and HONCH_SANDBOX_WIFI_PASSWORD"},
			{Key: "file", Value: "../SDK/ports/esp-idf/local/sdkconfig.defaults"},
		}))
	}
	endpoint, err := espIDFHardwareEndpoint(cfg, opts.DeviceEndpoint)
	if err != nil {
		return err
	}
	var build runner.EspIDFBuild
	if err := ui.WithSpinnerDone(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr(), "building ESP-IDF firmware for hardware", "ESP-IDF hardware firmware has been built", func(ctx context.Context) error {
		var buildErr error
		build, buildErr = r.Build(ctx, runner.EspIDFSettings{
			Endpoint:     endpoint,
			Token:        cfg.Sandbox.Token,
			WiFiSSID:     wifiSSID,
			WiFiPassword: wifiPassword,
			UseWiFi:      true,
		})
		return buildErr
	}); err != nil {
		return err
	}
	if err := saveRunnerSessionState(manager, session.RunnerState{Adapter: adapterConfig.Name, PID: os.Getpid(), Detached: false, ControlPath: ""}, runnerProxyState(cmd.Context(), cfg)); err != nil {
		return err
	}
	err = r.RunHardware(cmd.Context(), build, runner.HardwareRunSettings{
		Port:       opts.DevicePort,
		EraseFlash: opts.EraseFlash,
	}, cmd.OutOrStdout(), cmd.ErrOrStderr())
	if clearErr := clearForegroundRunnerState(manager); clearErr != nil {
		return errors.Join(err, clearErr)
	}
	return err
}

func sdkLocalWiFiDefaults(root string) (string, string, bool) {
	defaults, err := readSDKLocalDefaults(filepath.Join(root, "..", "SDK", "ports", "esp-idf", "local", "sdkconfig.defaults"))
	if err != nil {
		return "", "", false
	}
	ssid := defaults["CONFIG_WIFI_SSID"]
	password := defaults["CONFIG_WIFI_PASSWORD"]
	return ssid, password, ssid != "" && password != ""
}

func readSDKLocalDefaults(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		values[key] = value
	}
	return values, nil
}

func saveRunnerSessionState(manager session.Manager, runnerState session.RunnerState, proxyState session.ProxyState) error {
	state, err := loadSessionForUpdate(manager)
	if err != nil {
		return err
	}
	state.Runner = runnerState
	state.Proxy = proxyState
	return manager.Save(state)
}

func saveForegroundRunnerSessionState(manager session.Manager, runnerState session.RunnerState, proxyState session.ProxyState, cmd *exec.Cmd) error {
	state, err := loadSessionForUpdate(manager)
	if err != nil {
		if cmd != nil && cmd.Process != nil {
			_ = killProcess(cmd.Process.Pid)
			_ = cmd.Wait()
		}
		return err
	}
	state.Runner = runnerState
	state.Proxy = proxyState
	return saveForegroundRunnerState(manager, state, cmd)
}

func loadSessionForUpdate(manager session.Manager) (session.State, error) {
	state, err := manager.Load()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return session.State{}, nil
		}
		return session.State{}, err
	}
	return state, nil
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

func espIDFHardwareEndpoint(cfg config.Config, override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return strings.TrimRight(strings.TrimSpace(override), "/"), nil
	}
	ip, err := firstLANIPv4()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("http://%s:%d", ip, cfg.Ports.Proxy), nil
}

func firstLANIPv4() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch value := addr.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			if ip == nil {
				continue
			}
			ip = ip.To4()
			if ip == nil || ip.IsLoopback() {
				continue
			}
			return ip.String(), nil
		}
	}
	return "", fmt.Errorf("could not find a LAN IPv4 address; pass --device-endpoint")
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
		ProjectDir: filepath.Join(root, adapterConfig.Harness),
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
