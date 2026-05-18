package stack

import (
	"context"
	"fmt"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/config"
)

type Service struct {
	Root              string
	ApproveMigrations func() (bool, error)
	SkipMigrations    bool
}

func New(root string) Service {
	return Service{Root: root}
}

func (s Service) Start(ctx context.Context, cfg config.Config) error {
	runMigrations, err := s.shouldApplyPlatformMigrations(cfg)
	if err != nil {
		return err
	}
	foreground, background := splitCommands(cfg.Stack.StartCommands)
	if err := s.runCommands(ctx, cfg, foreground); err != nil {
		return err
	}
	if runMigrations || sandboxProjectConfigured(cfg) {
		if err := s.waitForPostgresReady(ctx, 30*time.Second); err != nil {
			return err
		}
	}
	if runMigrations {
		if err := s.applyPlatformMigrations(ctx, cfg); err != nil {
			return err
		}
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
