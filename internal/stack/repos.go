package stack

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/honch/sdk/tools/sandbox/internal/config"
)

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
