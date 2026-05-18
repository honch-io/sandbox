package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"honch.dev/honch/internal/config"
	"honch.dev/honch/internal/ui"
)

const (
	dockerImageInspectTimeout = 3 * time.Second
	dockerImagePullTimeout    = 10 * time.Minute
)

var confirmImagePull = ui.PromptConfirm
var shouldConfirmImagePull = func(stdin io.Reader, stderr io.Writer) bool {
	return ui.IsInteractive(stdin, stderr) && !ui.IsPlain()
}

func newImagesCommand(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "images",
		Short: "Manage required Docker images",
		Args:  rejectUnknownArgs,
		RunE:  commandGroupRunE,
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "Show required Docker image status",
			RunE: func(cmd *cobra.Command, args []string) error {
				_, cfg, _, err := loadRuntime(deps)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprint(cmd.OutOrStdout(), ui.FormatSections("Honch sandbox images", []ui.Section{
					{Name: "images", Rows: dockerImageRows(cmd.Context(), cfg)},
				}))
				return nil
			},
		},
		&cobra.Command{
			Use:   "pull",
			Short: "Pull required Docker images",
			RunE: func(cmd *cobra.Command, args []string) error {
				_, cfg, _, err := loadRuntime(deps)
				if err != nil {
					return err
				}
				return pullDockerImages(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr(), cfg.Stack.Images)
			},
		},
	)
	return cmd
}

func dockerImageRows(ctx context.Context, cfg config.Config) []ui.Row {
	images := uniqueDockerImages(cfg.Stack.Images)
	if len(images) == 0 {
		return []ui.Row{{Key: "status", Value: "none configured"}}
	}
	rows := make([]ui.Row, 0, len(images))
	for _, image := range images {
		rows = append(rows, ui.Row{Key: image, Value: dockerImageStatus(ctx, image)})
	}
	return rows
}

func missingDockerImages(ctx context.Context, cfg config.Config) []string {
	missing := []string{}
	for _, image := range uniqueDockerImages(cfg.Stack.Images) {
		if dockerImageStatus(ctx, image) != "present" {
			missing = append(missing, image)
		}
	}
	return missing
}

func dockerImageStatus(ctx context.Context, image string) string {
	if commandStatus("docker") == "missing" {
		return "docker missing"
	}
	inspectCtx, cancel := context.WithTimeout(ctx, dockerImageInspectTimeout)
	defer cancel()
	cmd := exec.CommandContext(inspectCtx, "docker", "image", "inspect", image)
	if err := cmd.Run(); err != nil {
		if errors.Is(inspectCtx.Err(), context.DeadlineExceeded) {
			return "docker unhealthy"
		}
		return "missing"
	}
	return "present"
}

func pullDockerImages(ctx context.Context, stdin io.Reader, stdout io.Writer, stderr io.Writer, images []string) error {
	for _, image := range uniqueDockerImages(images) {
		if err := pullDockerImage(ctx, stdin, stdout, stderr, image); err != nil {
			return err
		}
	}
	return nil
}

func pullDockerImage(ctx context.Context, stdin io.Reader, stdout io.Writer, stderr io.Writer, image string) error {
	image = strings.TrimSpace(image)
	if image == "" {
		return nil
	}
	if dockerImageStatus(ctx, image) == "present" && shouldConfirmImagePull(stdin, stderr) {
		ok, err := confirmImagePull(stdin, stderr, "You already have "+image+" locally. Pull anyway? [y/N] ")
		if err != nil {
			return err
		}
		if !ok {
			_, _ = fmt.Fprintf(stderr, "skipping %s\n", image)
			return nil
		}
	}
	return ui.WithSpinnerDone(ctx, stdin, stderr, "pulling "+image, "pulled "+image, func(ctx context.Context) error {
		pullCtx, cancel := context.WithTimeout(ctx, dockerImagePullTimeout)
		defer cancel()
		cmd := exec.CommandContext(pullCtx, "docker", "pull", image)
		out, err := cmd.CombinedOutput()
		if err != nil {
			if errors.Is(pullCtx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("docker pull timed out for %s", image)
			}
			if len(out) > 0 {
				return fmt.Errorf("docker pull failed for %s: %w\n%s", image, err, strings.TrimSpace(string(out)))
			}
			return fmt.Errorf("docker pull failed for %s: %w", image, err)
		}
		return nil
	})
}

func uniqueDockerImages(images []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(images))
	for _, image := range images {
		image = strings.TrimSpace(image)
		if image == "" || seen[image] {
			continue
		}
		seen[image] = true
		result = append(result, image)
	}
	return result
}
