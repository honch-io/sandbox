package stack

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/config"
)

type Service struct {
	Root              string
	ApproveMigrations func() (bool, error)
}

func New(root string) Service {
	return Service{Root: root}
}

var ErrMigrationDeclined = errors.New("platform migration declined")

func (s Service) Start(ctx context.Context, cfg config.Config) error {
	if err := s.confirmPlatformMigrations(cfg); err != nil {
		return err
	}
	foreground, background := splitCommands(cfg.Stack.StartCommands)
	if err := s.runCommands(ctx, cfg, foreground); err != nil {
		return err
	}
	if err := s.applyPlatformMigrations(ctx, cfg); err != nil {
		return err
	}
	if err := s.seedSandboxProject(ctx, cfg); err != nil {
		return err
	}
	if err := s.runCommands(ctx, cfg, background); err != nil {
		return err
	}
	return s.waitForBackgroundPorts(ctx, cfg, 30*time.Second)
}

func (s Service) Stop(ctx context.Context, cfg config.Config) error {
	if err := s.stopBackgroundProcesses(cfg); err != nil {
		return err
	}
	return s.runCommands(ctx, cfg, cfg.Stack.StopCommands)
}

func (s Service) Update(ctx context.Context, cfg config.Config) error {
	repos := repoMap(cfg)
	// Check every repo before fetching any of them. A dirty sibling repo should
	// stop the whole update without partially moving the local stack forward.
	for _, name := range repoNames() {
		path := repos[name]
		abs := s.resolve(path)
		if err := ensureClean(ctx, abs); err != nil {
			return fmt.Errorf("%s repo: %w", name, err)
		}
	}
	for _, name := range repoNames() {
		path := repos[name]
		abs := s.resolve(path)
		if err := run(ctx, abs, "git", "fetch", "origin"); err != nil {
			return fmt.Errorf("%s fetch: %w", name, err)
		}
		if err := run(ctx, abs, "git", "pull", "--ff-only"); err != nil {
			return fmt.Errorf("%s fast-forward: %w", name, err)
		}
	}
	return nil
}

func (s Service) Health(ctx context.Context, cfg config.Config) map[string]string {
	result := make(map[string]string)
	for name, path := range repoMap(cfg) {
		abs := s.resolve(path)
		if err := run(ctx, abs, "git", "rev-parse", "--show-toplevel"); err != nil {
			result[name] = "missing"
			continue
		}
		if err := ensureClean(ctx, abs); err != nil {
			result[name] = "dirty"
			continue
		}
		result[name] = "clean"
	}
	return result
}

func (s Service) runCommands(ctx context.Context, cfg config.Config, commands []config.CommandConfig) error {
	repos := repoMap(cfg)
	for _, command := range commands {
		if len(command.Args) == 0 {
			continue
		}
		repoPath, ok := repos[command.Repo]
		if !ok {
			return fmt.Errorf("unknown repo %q", command.Repo)
		}
		dir := s.resolveCommandDir(repoPath, command.WorkingDir)
		if command.Background {
			if err := s.startBackground(ctx, dir, cfg, command); err != nil {
				return fmt.Errorf("%s: %w", command.Repo, err)
			}
			continue
		}
		if err := run(ctx, dir, command.Args[0], command.Args[1:]...); err != nil {
			return fmt.Errorf("%s: %w", command.Repo, err)
		}
	}
	return nil
}

func (s Service) resolve(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Clean(filepath.Join(s.Root, path))
}

func (s Service) resolveCommandDir(repoPath string, workingDir string) string {
	dir := s.resolve(repoPath)
	if workingDir == "" {
		return dir
	}
	return filepath.Join(dir, workingDir)
}

func (s Service) startBackground(ctx context.Context, dir string, cfg config.Config, command config.CommandConfig) error {
	if s.backgroundAlreadyRunning(ctx, cfg, command.Repo) {
		return nil
	}
	cmd := exec.CommandContext(ctx, command.Args[0], command.Args[1:]...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	for key, val := range command.Env {
		cmd.Env = append(cmd.Env, key+"="+val)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var logFile *os.File
	if command.Log != "" {
		logPath := filepath.Join(s.Root, cfg.Sandbox.StateDir, "logs", command.Log)
		if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
			return err
		}
		var err error
		logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return err
		}
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}
	if err := cmd.Start(); err != nil {
		if logFile != nil {
			_ = logFile.Close()
		}
		return err
	}
	if logFile != nil {
		_ = logFile.Close()
	}
	return s.writePID(cfg, command.Repo, cmd.Process.Pid)
}

func (s Service) backgroundAlreadyRunning(ctx context.Context, cfg config.Config, repo string) bool {
	port := 0
	switch repo {
	case "capture":
		port = cfg.Ports.Capture
	case "worker":
		port = cfg.Ports.Worker
	default:
		return false
	}
	dialer := net.Dialer{Timeout: 200 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (s Service) waitForBackgroundPorts(ctx context.Context, cfg config.Config, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for _, check := range []struct {
		name string
		port int
	}{
		{name: "capture", port: cfg.Ports.Capture},
		{name: "worker", port: cfg.Ports.Worker},
	} {
		if check.port <= 0 {
			continue
		}
		for {
			if portOpen(ctx, check.port, 250*time.Millisecond) {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("%s did not become ready on port %d", check.name, check.port)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(250 * time.Millisecond):
			}
		}
	}
	return nil
}

func portOpen(ctx context.Context, port int, timeout time.Duration) bool {
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (s Service) writePID(cfg config.Config, name string, pid int) error {
	if name == "" {
		name = "process"
	}
	dir := filepath.Join(s.Root, cfg.Sandbox.StateDir, "pids")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".pid"), []byte(fmt.Sprintf("%d", pid)), 0o600)
}

func (s Service) stopBackgroundProcesses(cfg config.Config) error {
	pidDir := filepath.Join(s.Root, cfg.Sandbox.StateDir, "pids")
	entries, err := os.ReadDir(pidDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pid") {
			continue
		}
		path := filepath.Join(pidDir, entry.Name())
		data, readErr := os.ReadFile(path)
		if readErr == nil {
			var pid int
			if _, scanErr := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); scanErr == nil && pid > 0 {
				if killErr := syscall.Kill(-pid, syscall.SIGINT); killErr != nil {
					if process, findErr := os.FindProcess(pid); findErr == nil {
						_ = process.Signal(os.Interrupt)
					}
				}
			}
		}
		if removeErr := os.Remove(path); removeErr != nil {
			return removeErr
		}
	}
	return nil
}

func repoMap(cfg config.Config) map[string]string {
	return map[string]string{
		"capture":  cfg.Repos.Capture,
		"platform": cfg.Repos.Platform,
		"worker":   cfg.Repos.Worker,
	}
}

func repoNames() []string {
	return []string{"capture", "platform", "worker"}
}

func ensureClean(ctx context.Context, dir string) error {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("status failed: %s", strings.TrimSpace(out.String()))
	}
	if strings.TrimSpace(out.String()) != "" {
		return fmt.Errorf("dirty worktree")
	}
	return nil
}

func run(ctx context.Context, dir string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s failed: %s", name, strings.Join(args, " "), strings.TrimSpace(out.String()))
	}
	return nil
}

func splitCommands(commands []config.CommandConfig) ([]config.CommandConfig, []config.CommandConfig) {
	var foreground []config.CommandConfig
	var background []config.CommandConfig
	for _, command := range commands {
		if command.Background {
			background = append(background, command)
			continue
		}
		foreground = append(foreground, command)
	}
	return foreground, background
}

func (s Service) applyPlatformMigrations(ctx context.Context, cfg config.Config) error {
	if cfg.Repos.Platform == "" {
		return nil
	}
	if err := s.applyPostgresPrerequisites(ctx); err != nil {
		return err
	}
	backendDir := filepath.Join(s.resolve(cfg.Repos.Platform), "backend")
	return run(ctx, backendDir, "bun", "run", "db:migrate")
}

func (s Service) confirmPlatformMigrations(cfg config.Config) error {
	if cfg.Repos.Platform == "" {
		return nil
	}
	if s.ApproveMigrations == nil {
		return fmt.Errorf("platform migration approval is required")
	}
	approved, err := s.ApproveMigrations()
	if err != nil {
		return err
	}
	if !approved {
		return ErrMigrationDeclined
	}
	return nil
}

func (s Service) applyPostgresPrerequisites(ctx context.Context) error {
	return run(ctx, s.Root, "docker", "exec", "-i", "infra-postgres-1", "psql", "-U", "platform", "-d", "platform", "-c", PostgresPrerequisiteSQL())
}

func (s Service) seedSandboxProject(ctx context.Context, cfg config.Config) error {
	if cfg.Sandbox.ProjectID == "" || cfg.Sandbox.Token == "" {
		return nil
	}
	return run(ctx, s.Root, "docker", "exec", "-i", "infra-postgres-1", "psql", "-U", "platform", "-d", "platform", "-c", SandboxSeedSQL(cfg))
}

func SandboxSeedSQL(cfg config.Config) string {
	projectID := strings.ReplaceAll(cfg.Sandbox.ProjectID, "'", "''")
	token := strings.ReplaceAll(cfg.Sandbox.Token, "'", "''")
	return fmt.Sprintf(`
INSERT INTO organizations (id, name, slug)
VALUES ('00000000-0000-0000-0000-000000000001', 'Honch Sandbox', 'honch-sandbox')
ON CONFLICT (id) DO NOTHING;

INSERT INTO projects (id, name, api_key, organization_id)
VALUES ('%s', 'Sandbox Project', '%s', '00000000-0000-0000-0000-000000000001')
ON CONFLICT (id) DO UPDATE SET api_key = EXCLUDED.api_key, updated_at = now();
`, projectID, token)
}

func PostgresPrerequisiteSQL() string {
	return "CREATE EXTENSION IF NOT EXISTS pgcrypto;"
}
