package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
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
		return fmt.Errorf("%s failed: %s", name, out)
	}
	return nil
}
