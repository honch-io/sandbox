package commands

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/config"
	"github.com/honch/sdk/tools/sandbox/internal/proxy"
	"github.com/honch/sdk/tools/sandbox/internal/ui"
)

var lookupListeningProcess = identifyListeningProcess
var confirmProxyPortReuse = ui.PromptConfirm
var shouldConfirmProxyPortReuse = func(stdin io.Reader, stderr io.Writer) bool {
	return ui.IsInteractive(stdin, stderr) && !ui.IsPlain()
}
var stopProxyProcess = killProcess
var waitForProxyPortClose = waitForPortClose
var sandboxProcessIDsFn = sandboxProcessIDs
var killProcessFn = killProcess

func ensureControlFIFO(root string, cfg config.Config, adapter string) (string, error) {
	path := adapterControlPath(root, cfg, adapter)
	_ = os.Remove(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func proxyModePath(root string, cfg config.Config) string {
	return filepath.Join(root, cfg.Sandbox.StateDir, "proxy.mode")
}

func writeProxyMode(root string, cfg config.Config, mode proxy.Mode) error {
	path := proxyModePath(root, cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(mode.String()), 0o600)
}

func startProxyProcess(ctx context.Context, root string, cfg config.Config, stdin io.Reader, stdout io.Writer, stderr io.Writer) (*os.Process, error) {
	if portIsOpen(context.Background(), cfg.Ports.Proxy, 200*time.Millisecond) {
		if pid, ok := readPID(proxyPIDPath(root, cfg)); ok && processAlive(pid) && processCommandContains(pid, "sandbox proxy-serve") {
			_ = appendProxyLog(root, cfg, fmt.Sprintf("proxy already running on 127.0.0.1:%d\n", cfg.Ports.Proxy))
			return nil, nil
		}
		return nil, fmt.Errorf("proxy port 127.0.0.1:%d is already in use by a non-sandbox process", cfg.Ports.Proxy)
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	logPath := filepath.Join(root, cfg.Sandbox.StateDir, "logs", "proxy.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe, "sandbox", "proxy-serve")
	cmd.Dir = root
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, err
	}
	proc, err := releaseDetachedProcess(cmd, func(pid int) error {
		return waitForProxyReadyAndWritePID(context.Background(), root, cfg, pid, 5*time.Second)
	})
	if err != nil {
		_ = logFile.Close()
		return nil, err
	}
	_ = logFile.Close()
	return proc, nil
}

func resolveProxyPortConflict(ctx context.Context, stdin io.Reader, stderr io.Writer, root string, cfg config.Config) error {
	if pid, ok := readPID(proxyPIDPath(root, cfg)); ok && processAlive(pid) && processCommandContains(pid, "sandbox proxy-serve") {
		_ = appendProxyLog(root, cfg, fmt.Sprintf("proxy already running on 127.0.0.1:%d\n", cfg.Ports.Proxy))
		return nil
	}
	if !portIsOpen(ctx, cfg.Ports.Proxy, 200*time.Millisecond) {
		return nil
	}
	occupant, ok := lookupListeningProcess(cfg.Ports.Proxy)
	if !ok {
		return fmt.Errorf("proxy port 127.0.0.1:%d is already in use by a non-sandbox process", cfg.Ports.Proxy)
	}
	if !shouldConfirmProxyPortReuse(stdin, stderr) {
		return fmt.Errorf("proxy port 127.0.0.1:%d is already in use by %s", cfg.Ports.Proxy, occupant.summary())
	}
	ok, err := confirmProxyPortReuse(stdin, stderr, proxyPortReusePrompt(cfg.Ports.Proxy, occupant, root))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("start cancelled")
	}
	if processAlive(occupant.PID) {
		if err := stopProxyProcess(occupant.PID); err != nil {
			return err
		}
	}
	if err := waitForProxyPortClose(ctx, cfg.Ports.Proxy, 5*time.Second); err != nil {
		return err
	}
	return nil
}

type listeningProcessInfo struct {
	PID     int
	Command string
	Cwd     string
}

func (i listeningProcessInfo) summary() string {
	switch {
	case i.Command != "" && i.Cwd != "":
		return i.Command + " (" + i.Cwd + ")"
	case i.Command != "":
		return i.Command
	case i.Cwd != "":
		return i.Cwd
	default:
		return "an unknown process"
	}
}

func proxyPortReusePrompt(port int, occupant listeningProcessInfo, root string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("proxy port 127.0.0.1:%d is already in use.\n\n", port))
	b.WriteString(fmt.Sprintf("  pid: %d\n", occupant.PID))
	if occupant.Command != "" {
		b.WriteString(fmt.Sprintf("  command: %s\n", occupant.Command))
	}
	if occupant.Cwd != "" {
		b.WriteString(fmt.Sprintf("  cwd: %s\n", occupant.Cwd))
	}
	if occupant.Cwd != "" && strings.Contains(occupant.Cwd, root) {
		b.WriteString("\n  this looks like the current checkout\n")
	}
	b.WriteString("\nStop it and continue starting the sandbox? [y/N] ")
	return b.String()
}

func identifyListeningProcess(port int) (listeningProcessInfo, bool) {
	pid, ok := listeningPortPID(port)
	if !ok {
		return listeningProcessInfo{}, false
	}
	info := listeningProcessInfo{PID: pid, Command: processCommandLine(pid), Cwd: processWorkingDirectory(pid)}
	return info, true
}

func listeningPortPID(port int) (int, bool) {
	out, err := exec.Command("lsof", "-nP", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN", "-Fp").Output()
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "p") {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimPrefix(line, "p"))
		if err == nil && pid > 0 {
			return pid, true
		}
	}
	return 0, false
}

func processCommandLine(pid int) string {
	if pid <= 0 {
		return ""
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func processWorkingDirectory(pid int) string {
	if pid <= 0 {
		return ""
	}
	out, err := exec.Command("lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "n") {
			return strings.TrimPrefix(line, "n")
		}
	}
	return ""
}

func waitForPortClose(ctx context.Context, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !portIsOpen(ctx, port, 100*time.Millisecond) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("proxy port 127.0.0.1:%d did not close within %s", port, timeout)
}

func waitForProxyReadyAndWritePID(ctx context.Context, root string, cfg config.Config, pid int, timeout time.Duration) error {
	if err := waitForPortReady(ctx, cfg.Ports.Proxy, pid, timeout); err != nil {
		return err
	}
	return writePIDFile(proxyPIDPath(root, cfg), pid)
}

func proxyPIDPath(root string, cfg config.Config) string {
	return filepath.Join(root, cfg.Sandbox.StateDir, "pids", "proxy.pid")
}

func appendProxyLog(root string, cfg config.Config, line string) error {
	logPath := filepath.Join(root, cfg.Sandbox.StateDir, "logs", "proxy.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	_, err = fmt.Fprint(logFile, line)
	return err
}

func portIsOpen(ctx context.Context, port int, timeout time.Duration) bool {
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func startRunnerSupervisor(root string, cfg config.Config, adapter string, target string, controlPath string, env map[string]string, afterReady func(*os.Process) error) (*os.Process, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	logPath := filepath.Join(root, cfg.Sandbox.StateDir, "logs", "device.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe, "sandbox", "runner-serve", adapter, target, controlPath)
	cmd.Dir = root
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, err
	}
	proc, err := releaseDetachedProcess(cmd, func(pid int) error {
		if err := waitForRunnerReady(context.Background(), logPath, pid, runnerReadyTimeout()); err != nil {
			return err
		}
		if afterReady != nil {
			return afterReady(cmd.Process)
		}
		return nil
	})
	if err != nil {
		_ = logFile.Close()
		return nil, err
	}
	_ = logFile.Close()
	return proc, nil
}

func releaseDetachedProcess(cmd *exec.Cmd, waitReady func(pid int) error) (*os.Process, error) {
	proc := cmd.Process
	pid := proc.Pid
	if err := waitReady(pid); err != nil {
		_ = killProcess(pid)
		_ = cmd.Wait()
		return nil, err
	}
	if err := proc.Release(); err != nil {
		_ = killProcess(pid)
		_ = cmd.Wait()
		return nil, err
	}
	return proc, nil
}

func killProcess(pid int) error {
	if pid <= 0 {
		return nil
	}
	// Detached sandbox processes run in their own process groups. Signal the
	// group first so supervised children stop with their parent.
	if err := syscall.Kill(-pid, syscall.SIGINT); err == nil {
		return nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(os.Interrupt)
}

func killSandboxRunnerProcesses(root string, cfg config.Config) error {
	return killSandboxProcessesByPatterns(sandboxRunnerProcessPatterns(root, cfg))
}

func sandboxRunnerProcessPatterns(root string, cfg config.Config) []string {
	buildBinary := filepath.Join(root, cfg.Sandbox.StateDir, "build", "c-core", "honch_sandbox_c_core")
	return []string{
		buildBinary,
		filepath.Join(root, "honch") + " sandbox runner-serve ",
		"idf.py -B " + filepath.Join(root, cfg.Sandbox.StateDir, "build", "esp-idf") + " qemu",
		"qemu-system-xtensa .*" + filepath.Join(root, cfg.Sandbox.StateDir, "build", "esp-idf", "qemu_flash.bin"),
	}
}

func sandboxAdapterProcessPatterns(root string, cfg config.Config, adapter string) []string {
	switch adapter {
	case "c-core":
		return []string{
			filepath.Join(root, cfg.Sandbox.StateDir, "build", "c-core", "honch_sandbox_c_core"),
			filepath.Join(root, "honch") + " sandbox runner-serve c-core ",
		}
	case "esp-idf":
		return append([]string{
			filepath.Join(root, "honch") + " sandbox runner-serve esp-idf ",
		}, sandboxQEMUProcessPatterns(root, cfg)...)
	default:
		return []string{filepath.Join(root, "honch") + " sandbox runner-serve " + adapter + " "}
	}
}

func sandboxQEMUProcessPatterns(root string, cfg config.Config) []string {
	return []string{
		"idf.py -B " + filepath.Join(root, cfg.Sandbox.StateDir, "build", "esp-idf") + " qemu",
		"qemu-system-xtensa .*" + filepath.Join(root, cfg.Sandbox.StateDir, "build", "esp-idf", "qemu_flash.bin"),
	}
}

func sandboxStopProcessPatterns(root string, cfg config.Config) []string {
	return append([]string{
		filepath.Join(root, "honch") + " sandbox proxy-serve",
	}, sandboxRunnerProcessPatterns(root, cfg)...)
}

func killSandboxProcessesByPatterns(patterns []string) error {
	for _, pattern := range patterns {
		for _, pid := range sandboxProcessIDsFn(pattern) {
			_ = killProcessFn(pid)
		}
	}
	return nil
}

func adapterControlPath(root string, cfg config.Config, adapter string) string {
	return filepath.Join(root, cfg.Sandbox.StateDir, adapter+".control")
}

func sandboxHasMatchingProcesses(patterns []string) bool {
	for _, pattern := range patterns {
		if len(sandboxProcessIDsFn(pattern)) > 0 {
			return true
		}
	}
	return false
}

func sandboxProcessIDs(pattern string) []int {
	out, err := exec.Command("pgrep", "-f", pattern).Output()
	if err != nil {
		return nil
	}
	pids := []int{}
	for _, field := range strings.Fields(string(out)) {
		pid, scanErr := strconv.Atoi(field)
		if scanErr != nil || pid == os.Getpid() {
			continue
		}
		pids = append(pids, pid)
	}
	return pids
}

func readPID(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

func writePIDFile(path string, pid int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o600)
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func processCommandContains(pid int, fragment string) bool {
	if pid <= 0 || fragment == "" {
		return false
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), fragment)
}

func waitForPortReady(ctx context.Context, port int, pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return fmt.Errorf("sandbox process %d exited before port %d became ready", pid, port)
		}
		if portIsOpen(ctx, port, 100*time.Millisecond) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("sandbox process %d did not open port %d within %s", pid, port, timeout)
}

func waitForRunnerReady(ctx context.Context, logPath string, pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return fmt.Errorf("sandbox runner %d exited before reporting ready", pid)
		}
		data, err := os.ReadFile(logPath)
		if err == nil && strings.Contains(string(data), `"ready":true`) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("sandbox runner %d did not report ready within %s", pid, timeout)
}

func runnerReadyTimeout() time.Duration {
	if value := os.Getenv("HONCH_SANDBOX_RUNNER_READY_TIMEOUT"); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 30 * time.Second
}
