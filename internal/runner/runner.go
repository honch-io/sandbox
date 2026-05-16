package runner

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type CCoreRunner struct {
	RepoRoot string
	StateDir string
}

func (r CCoreRunner) Build(ctx context.Context) (string, error) {
	buildDir := filepath.Join(r.StateDir, "build", "c-core")
	sourceDir := filepath.Join(r.RepoRoot, "tools", "sandbox", "harnesses", "c-core")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return "", err
	}
	if err := run(ctx, "", "cmake", "-S", sourceDir, "-B", buildDir); err != nil {
		return "", err
	}
	if err := run(ctx, "", "cmake", "--build", buildDir); err != nil {
		return "", err
	}
	return filepath.Join(buildDir, "honch_sandbox_c_core"), nil
}

func (r CCoreRunner) Run(ctx context.Context, binary string, detach bool, env map[string]string, stdout io.Writer, stderr io.Writer) (*exec.Cmd, error) {
	cmd, err := Start(ctx, binary, env, os.Stdin, stdout, stderr)
	if err != nil {
		return nil, err
	}
	if !detach {
		return cmd, cmd.Wait()
	}
	return cmd, nil
}

type EspIDFSettings struct {
	Endpoint string
	Token    string
}

type EspIDFBuild struct {
	ProjectDir string
	BuildDir   string
}

type EspIDFRunner struct {
	RepoRoot string
	StateDir string
	IDFPath  string
}

func (r EspIDFRunner) Build(ctx context.Context, settings EspIDFSettings) (EspIDFBuild, error) {
	build := EspIDFBuild{
		ProjectDir: filepath.Join(r.RepoRoot, "tools", "sandbox", "harnesses", "esp-idf"),
		BuildDir:   filepath.Join(r.StateDir, "build", "esp-idf"),
	}
	// The ESP-IDF sandbox build directory is fully managed by this runner. A
	// failed configure can leave a partial directory that idf.py refuses to
	// fullclean, so start each build from a clean state.
	if err := os.RemoveAll(build.BuildDir); err != nil {
		return EspIDFBuild{}, err
	}
	if err := os.MkdirAll(build.BuildDir, 0o755); err != nil {
		return EspIDFBuild{}, err
	}
	args := espIDFBuildArgs(build.BuildDir, settings)
	if err := r.runIDF(ctx, build.ProjectDir, append(args, "set-target", "esp32")...); err != nil {
		return EspIDFBuild{}, err
	}
	if err := r.runIDF(ctx, build.ProjectDir, append(args, "build")...); err != nil {
		return EspIDFBuild{}, err
	}
	return build, nil
}

func (r EspIDFRunner) Run(ctx context.Context, build EspIDFBuild, controlPath string, stdout io.Writer, stderr io.Writer) error {
	return r.RunQEMU(ctx, build, controlPath, stdout, stderr)
}

func (r EspIDFRunner) RunQEMU(ctx context.Context, build EspIDFBuild, controlPath string, stdout io.Writer, stderr io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if err := r.prepareQEMUImages(ctx, build); err != nil {
		return err
	}
	qemu, err := r.startQEMU(ctx, build, stderr)
	if err != nil {
		return err
	}
	conn, err := dialQEMUSerial(ctx, qemuSerialAddr())
	if err != nil {
		killAndWait(qemu)
		return err
	}
	defer conn.Close()
	if controlPath != "" {
		go bridgeControlToWriter(ctx, controlPath, conn)
	}
	readyDone := make(chan struct{}, 1)
	copyDone := make(chan error, 1)
	go func() {
		copyDone <- copyQEMUSerial(stdout, conn, readyDone)
	}()
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- qemu.Wait()
	}()
	readyTimeout := qemuReadyTimeout()
	select {
	case <-ctx.Done():
		killAndWaitDone(qemu, waitDone)
		return ctx.Err()
	case err := <-waitDone:
		return fmt.Errorf("QEMU exited before firmware ready: %w", err)
	case err := <-copyDone:
		killAndWaitDone(qemu, waitDone)
		if err != nil {
			return fmt.Errorf("QEMU serial closed before firmware ready: %w", err)
		}
		return fmt.Errorf("QEMU serial closed before firmware ready")
	case <-readyDone:
	case <-time.After(readyTimeout):
		killAndWaitDone(qemu, waitDone)
		return fmt.Errorf("firmware did not report ready within %s", readyTimeout)
	}
	select {
	case <-ctx.Done():
		killAndWaitDone(qemu, waitDone)
		return ctx.Err()
	case err := <-waitDone:
		return err
	case err := <-copyDone:
		killAndWaitDone(qemu, waitDone)
		return err
	}
}

func killAndWait(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}

func killAndWaitDone(cmd *exec.Cmd, waitDone <-chan error) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
	}
}

func copyQEMUSerial(stdout io.Writer, src io.Reader, readyDone chan<- struct{}) error {
	reader := bufio.NewReader(src)
	readySent := false
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			if _, writeErr := io.WriteString(stdout, line); writeErr != nil {
				return writeErr
			}
			if !readySent && qemuReadyLine(line) {
				readySent = true
				readyDone <- struct{}{}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func qemuReadyLine(line string) bool {
	return strings.Contains(line, `"ready":true`) || strings.TrimSpace(line) == "ready"
}

func qemuReadyTimeout() time.Duration {
	if value := os.Getenv("HONCH_SANDBOX_QEMU_READY_TIMEOUT"); value != "" {
		if timeout, err := time.ParseDuration(value); err == nil && timeout > 0 {
			return timeout
		}
	}
	return 30 * time.Second
}

func (r EspIDFRunner) runIDF(ctx context.Context, dir string, args ...string) error {
	cmd := r.idfCommand(ctx, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			return fmt.Errorf("idf.py failed: %w: %s", err, out)
		}
		return fmt.Errorf("idf.py failed: %w", err)
	}
	return nil
}

func (r EspIDFRunner) prepareQEMUImages(ctx context.Context, build EspIDFBuild) error {
	flashPath := filepath.Join(build.BuildDir, "qemu_flash.bin")
	if err := r.runExported(ctx, build.BuildDir, "python", "-m", "esptool", "--chip=esp32", "merge-bin", "--output="+flashPath, "--pad-to-size=2MB", "@flash_args"); err != nil {
		return err
	}
	efusePath := filepath.Join(build.BuildDir, "qemu_efuse.bin")
	if _, err := os.Stat(efusePath); err == nil {
		return nil
	}
	efuse, err := hex.DecodeString("00000000000000000000000000800000000000000000100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")
	if err != nil {
		return err
	}
	return os.WriteFile(efusePath, efuse, 0o644)
}

func (r EspIDFRunner) startQEMU(ctx context.Context, build EspIDFBuild, stderr io.Writer) (*exec.Cmd, error) {
	args := []string{
		"-M", "esp32",
		"-m", "4M",
		"-drive", "file=" + filepath.Join(build.BuildDir, "qemu_flash.bin") + ",if=mtd,format=raw",
		"-drive", "file=" + filepath.Join(build.BuildDir, "qemu_efuse.bin") + ",if=none,format=raw,id=efuse",
		"-global", "driver=nvram.esp32.efuse,property=drive,value=efuse",
		"-global", "driver=timer.esp32.timg,property=wdt_disable,value=true",
		"-nographic",
		"-serial", qemuSerialArg(),
		"-monitor", "none",
		"-nic", "user,model=open_eth",
	}
	cmd := r.exportedCommand(ctx, "qemu-system-xtensa", args...)
	cmd.Dir = build.ProjectDir
	cmd.Stdout = stderr
	cmd.Stderr = stderr
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func qemuSerialAddr() string {
	if value := os.Getenv("HONCH_SANDBOX_QEMU_SERIAL_ADDR"); value != "" {
		return value
	}
	return "127.0.0.1:5555"
}

func qemuSerialArg() string {
	address := qemuSerialAddr()
	_, port, ok := strings.Cut(address, ":")
	if !ok || port == "" {
		port = "5555"
	}
	return "tcp::" + port + ",server,nowait"
}

func (r EspIDFRunner) runExported(ctx context.Context, dir string, name string, args ...string) error {
	cmd := r.exportedCommand(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			return fmt.Errorf("%s failed: %w: %s", name, err, out)
		}
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return nil
}

func (r EspIDFRunner) exportedCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	if r.IDFPath == "" {
		return exec.CommandContext(ctx, name, args...)
	}
	shellArgs := append([]string{". " + shellQuote(filepath.Join(r.IDFPath, "export.sh")) + " >/dev/null && exec " + shellQuote(name)}, shellQuoteArgs(args)...)
	return exec.CommandContext(ctx, "bash", "-lc", strings.Join(shellArgs, " "))
}

func (r EspIDFRunner) idfCommand(ctx context.Context, args ...string) *exec.Cmd {
	if r.IDFPath == "" {
		return exec.CommandContext(ctx, "idf.py", args...)
	}
	shellArgs := append([]string{". " + shellQuote(filepath.Join(r.IDFPath, "export.sh")) + " >/dev/null && idf.py"}, shellQuoteArgs(args)...)
	return exec.CommandContext(ctx, "bash", "-lc", strings.Join(shellArgs, " "))
}

func dialQEMUSerial(ctx context.Context, address string) (net.Conn, error) {
	var lastErr error
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var dialer net.Dialer
		conn, err := dialer.DialContext(ctx, "tcp", address)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return nil, fmt.Errorf("connect to QEMU serial socket %s: %w", address, lastErr)
}

func shellQuoteArgs(args []string) []string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return quoted
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func espIDFBuildArgs(buildDir string, settings EspIDFSettings) []string {
	return []string{
		"-B", buildDir,
		"-D", "SDKCONFIG=" + filepath.Join(buildDir, "sdkconfig"),
		"-D", "HONCH_SANDBOX_HOST=" + settings.Endpoint,
		"-D", "HONCH_SANDBOX_API_KEY=" + settings.Token,
	}
}

func Start(ctx context.Context, binary string, env map[string]string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, binary)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if stdin == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func bridgeControlToWriter(ctx context.Context, controlPath string, dst io.WriteCloser) {
	defer dst.Close()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		file, err := os.Open(controlPath)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			if _, err := fmt.Fprintln(dst, scanner.Text()); err != nil {
				_ = file.Close()
				return
			}
		}
		_ = file.Close()
	}
}

func SendControl(w io.Writer, action string, fields map[string]any) error {
	payload := map[string]any{"action": action}
	for key, value := range fields {
		payload[key] = value
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

func run(ctx context.Context, dir string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			return fmt.Errorf("%s failed: %w: %s", name, err, out)
		}
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return nil
}
