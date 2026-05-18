package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"honch.dev/honch/internal/config"
	"honch.dev/honch/internal/ui"
)

type siblingRepoSource struct {
	Name string
	URL  string
	Path string
}

func onboardingRepoRows(root string, cfg config.Config) []ui.Row {
	rows := []ui.Row{}
	for _, field := range []configField{
		configFieldByKey["repos.capture"],
		configFieldByKey["repos.platform"],
		configFieldByKey["repos.worker"],
	} {
		rows = append(rows, ui.Row{
			Key:   field.Name,
			Value: onboardingRepoStatus(root, fmt.Sprint(field.Read(cfg))),
		})
	}
	return rows
}

func onboardingRepoStatus(root string, raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "<not set>"
	}
	path := resolveSandboxPath(root, value)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "missing: " + value
		}
		return "unavailable: " + err.Error()
	}
	if !info.IsDir() {
		return "not a directory: " + value
	}
	return "ready: " + path
}

func needsRepoUpdate(rows []ui.Row) bool {
	for _, row := range rows {
		value := fmt.Sprint(row.Value)
		if value == "missing" || value == "dirty" || strings.Contains(value, "missing:") || strings.Contains(value, "not set") || strings.Contains(value, "unavailable:") || strings.Contains(value, "not a directory:") {
			return true
		}
	}
	return false
}

func cloneMissingSiblingRepos(ctx context.Context, prompts *ui.PromptSession, stdout io.Writer, stderr io.Writer, root string, cfg *config.Config) error {
	missing := missingSiblingRepoSources(root, *cfg)
	if len(missing) == 0 {
		return nil
	}
	parent, err := prompts.Text("Clone destination parent", filepath.Dir(root))
	if err != nil {
		return err
	}
	parent = strings.TrimSpace(parent)
	if parent == "" {
		parent = filepath.Dir(root)
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return err
	}
	if _, err := ensureConfigFile(root, *cfg); err != nil {
		return err
	}
	for _, source := range missing {
		target := filepath.Join(parent, source.Name)
		if err := cloneSiblingRepo(ctx, stdout, stderr, source, target); err != nil {
			return err
		}
		if err := saveRepoPath(root, *cfg, "repos."+source.Name, target); err != nil {
			return err
		}
		switch source.Name {
		case "capture":
			cfg.Repos.Capture = target
		case "platform":
			cfg.Repos.Platform = target
		case "worker":
			cfg.Repos.Worker = target
		}
	}
	return nil
}

func missingSiblingRepoSources(root string, cfg config.Config) []siblingRepoSource {
	sources := []siblingRepoSource{
		{Name: "capture", URL: cfg.RepoSources.Capture, Path: cfg.Repos.Capture},
		{Name: "platform", URL: cfg.RepoSources.Platform, Path: cfg.Repos.Platform},
		{Name: "worker", URL: cfg.RepoSources.Worker, Path: cfg.Repos.Worker},
	}
	missing := make([]siblingRepoSource, 0, len(sources))
	for _, source := range sources {
		if strings.TrimSpace(source.URL) == "" {
			continue
		}
		if strings.TrimSpace(source.Path) == "" {
			missing = append(missing, source)
			continue
		}
		path := resolveSandboxPath(root, source.Path)
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			continue
		}
		missing = append(missing, source)
	}
	return missing
}

func runSiblingRepoClone(ctx context.Context, stdout io.Writer, stderr io.Writer, source siblingRepoSource, target string) error {
	if commandStatus("git") == "missing" {
		return errors.New("git is required to clone Honch sibling repos")
	}
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("%s already exists", target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "$ git clone %s %s\n", source.URL, target)
	cmd := exec.CommandContext(ctx, "git", "clone", source.URL, target)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func saveRepoPath(root string, cfg config.Config, key string, value string) error {
	field, ok := configFieldByKey[key]
	if !ok {
		return fmt.Errorf("unsupported repo key %q", key)
	}
	return setConfigValue(root, cfg, field, value)
}

func promptAndSaveRepoPaths(prompts *ui.PromptSession, root string, cfg *config.Config) error {
	if _, err := ensureConfigFile(root, *cfg); err != nil {
		return err
	}
	for _, field := range []configField{
		configFieldByKey["repos.capture"],
		configFieldByKey["repos.platform"],
		configFieldByKey["repos.worker"],
	} {
		current := strings.TrimSpace(fmt.Sprint(field.Read(*cfg)))
		if current == "" {
			current = fmt.Sprint(field.Read(config.Config{}))
		}
		value, err := prompts.Text("Set "+field.Name+" repo path", current)
		if err != nil {
			return err
		}
		value = strings.TrimSpace(value)
		if value == "" || value == current {
			continue
		}
		if err := setConfigValue(root, *cfg, field, value); err != nil {
			return err
		}
		switch field.Key {
		case "repos.capture":
			cfg.Repos.Capture = value
		case "repos.platform":
			cfg.Repos.Platform = value
		case "repos.worker":
			cfg.Repos.Worker = value
		}
	}
	return nil
}
