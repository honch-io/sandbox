package stack

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/honch/sdk/tools/sandbox/internal/config"
)

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
