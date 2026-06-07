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

	"honch.dev/honch/internal/config"
	"honch.dev/honch/internal/proxy"
	"honch.dev/honch/internal/ui"
)

var lookupListeningProcess = identifyListeningProcess
var confirmProxyPortReuse = ui.PromptConfirm
var shouldConfirmProxyPortReuse = func(stdin io.Reader, stderr io.Writer) bool {
	return ui.IsInteractive(stdin, stderr) && !ui.IsPlain()
}
var stopProxyProcess = killProcess
var waitForProxyPortClose = waitForPortClose

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

func proxyProbeHost(cfg config.Config) string {
	switch cfg.Sandbox.ProxyBind {
	case "", "0.0.0.0", "::":
		return "127.0.0.1"
	default:
		return cfg.Sandbox.ProxyBind
	}
}

func proxyAddrLabel(cfg config.Config) string {
	return net.JoinHostPort(proxyProbeHost(cfg), strconv.Itoa(cfg.Ports.Proxy))
}

func startProxyProcess(ctx context.Context, root string, cfg config.Config, stdin io.Reader, stdout io.Writer, stderr io.Writer) (*os.Process, error) {
	if portIsOpenOn(context.Background(), proxyProbeHost(cfg), cfg.Ports.Proxy, 200*time.Millisecond) {
		if pid, ok := readPID(proxyPIDPath(root, cfg)); ok && processAlive(pid) && processCommandContains(pid, "sandbox proxy-serve") {
			_ = appendProxyLog(root, cfg, fmt.Sprintf("proxy already running on %s\n", proxyAddrLabel(cfg)))
			return nil, nil
		}
		return nil, fmt.Errorf("proxy port %s is already in use by a non-sandbox process", proxyAddrLabel(cfg))
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
		_ = appendProxyLog(root, cfg, fmt.Sprintf("proxy already running on %s\n", proxyAddrLabel(cfg)))
		return nil
	}
	if !portIsOpenOn(ctx, proxyProbeHost(cfg), cfg.Ports.Proxy, 200*time.Millisecond) {
		return nil
	}
	occupant, ok := lookupListeningProcess(cfg.Ports.Proxy)
	if !ok {
		return fmt.Errorf("proxy port %s is already in use by a non-sandbox process", proxyAddrLabel(cfg))
	}
	if !shouldConfirmProxyPortReuse(stdin, stderr) {
		return fmt.Errorf("proxy port %s is already in use by %s", proxyAddrLabel(cfg), occupant.summary())
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
	if err := waitForPortReadyOn(ctx, proxyProbeHost(cfg), cfg.Ports.Proxy, pid, timeout); err != nil {
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
