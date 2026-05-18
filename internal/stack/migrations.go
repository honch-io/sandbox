package stack

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/config"
)

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

func (s Service) shouldApplyPlatformMigrations(cfg config.Config) (bool, error) {
	if cfg.Repos.Platform == "" || s.SkipMigrations {
		return false, nil
	}
	if s.ApproveMigrations == nil {
		return false, fmt.Errorf("platform migration approval is required")
	}
	approved, err := s.ApproveMigrations()
	if err != nil {
		return false, err
	}
	if !approved {
		return false, nil
	}
	return true, nil
}

func (s Service) applyPostgresPrerequisites(ctx context.Context) error {
	return run(ctx, s.Root, "docker", "exec", "-i", "infra-postgres-1", "psql", "-U", "platform", "-d", "platform", "-c", PostgresPrerequisiteSQL())
}

func (s Service) seedSandboxProject(ctx context.Context, cfg config.Config) error {
	if !sandboxProjectConfigured(cfg) {
		return nil
	}
	return run(ctx, s.Root, "docker", "exec", "-i", "infra-postgres-1", "psql", "-U", "platform", "-d", "platform", "-c", SandboxSeedSQL(cfg))
}

func sandboxProjectConfigured(cfg config.Config) bool {
	return cfg.Sandbox.ProjectID != "" && cfg.Sandbox.Token != ""
}

func (s Service) waitForPostgresReady(ctx context.Context, timeout time.Duration) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	for {
		if err := run(ctx, s.Root, "docker", "exec", "-i", "infra-postgres-1", "pg_isready", "-U", "platform", "-d", "platform"); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("postgres did not become ready within %s", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
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
