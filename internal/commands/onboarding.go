package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/config"
	"github.com/honch/sdk/tools/sandbox/internal/ui"
	"github.com/spf13/cobra"
)

const onboardingStateVersion = 1

var errOnboardingExited = errors.New("onboarding exited")

var onboardingGate = defaultOnboardingGate
var cloneSiblingRepo = runSiblingRepoClone

type onboardingState struct {
	Version     int       `json:"version"`
	CompletedAt time.Time `json:"completed_at"`
}

func defaultOnboardingGate(in io.Reader, out io.Writer) bool {
	return ui.IsInteractive(in, out) && !ui.IsPlain()
}

func newOnboardingCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "onboarding",
		Short: "Run the first-launch setup wizard",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, _, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			err = runOnboardingWizard(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr(), root, cfg)
			if errors.Is(err, errOnboardingExited) {
				return nil
			}
			return err
		},
	}
}

func maybeRunOnboarding(cmd *cobra.Command, deps Dependencies) error {
	if !onboardingGate(cmd.InOrStdin(), cmd.OutOrStdout()) {
		return nil
	}
	if shouldSkipAutoOnboarding(cmd) {
		return nil
	}
	root, cfg, _, err := loadRuntime(deps)
	if err != nil {
		return err
	}
	done, err := onboardingComplete(root, cfg)
	if err != nil {
		return err
	}
	if done {
		return nil
	}
	return runOnboardingWizard(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr(), root, cfg)
}

func shouldSkipAutoOnboarding(cmd *cobra.Command) bool {
	switch cmd.CommandPath() {
	case "honch install", "honch onboarding":
		return true
	default:
		return false
	}
}

func runOnboardingWizard(ctx context.Context, stdin io.Reader, stdout io.Writer, stderr io.Writer, root string, cfg config.Config) error {
	prompts := ui.NewPromptSession(stdin, stdout)
	report := buildSandboxDoctorReport(root, cfg)
	target, err := defaultInstallTarget()
	if err != nil {
		return err
	}

	step := 1
	for {
		switch step {
		case 1:
			printOnboardingStep(stdout, 1, 4, "Welcome", []ui.Section{{
				Rows: []ui.Row{
					{Key: "workspace", Value: root},
					{Key: "config", Value: configFilePath(root)},
					{Key: "this wizard", Value: "connects required repos, prepares the local stack, and can install the honch binary"},
				},
			}})
			action, err := prompts.ContinueOrExit("Continue onboarding?")
			if err != nil {
				return err
			}
			if action == ui.PromptActionExit {
				return exitOnboarding(stdout)
			}
			step = 2
		case 2:
			printOnboardingStep(stdout, 2, 4, "Repositories", []ui.Section{{
				Name: "current paths",
				Rows: onboardingRepoRows(root, cfg),
			}})
			if needsRepoUpdate(report.Repos) {
				action, err := prompts.ConfirmNavigate("Clone missing Honch repos now?", false, true)
				if err != nil {
					return err
				}
				switch action {
				case ui.PromptActionBack:
					step = 1
					continue
				case ui.PromptActionExit:
					return exitOnboarding(stdout)
				case ui.PromptActionYes:
					if err := cloneMissingSiblingRepos(ctx, prompts, stdout, stderr, root, &cfg); err != nil {
						return err
					}
				case ui.PromptActionNo:
					action, err := prompts.ConfirmNavigate("Update sibling repo paths now?", false, true)
					if err != nil {
						return err
					}
					switch action {
					case ui.PromptActionBack:
						step = 2
						continue
					case ui.PromptActionExit:
						return exitOnboarding(stdout)
					case ui.PromptActionYes:
						if err := promptAndSaveRepoPaths(prompts, root, &cfg); err != nil {
							return err
						}
					}
				}
				report = buildSandboxDoctorReport(root, cfg)
			}
			step = 3
		case 3:
			printOnboardingStep(stdout, 3, 4, "Setup", []ui.Section{{
				Name: "recommended fixes",
				Rows: onboardingSetupRows(report),
			}})
			action, err := prompts.ConfirmNavigate("Run the recommended sandbox setup now?", false, true)
			if err != nil {
				return err
			}
			switch action {
			case ui.PromptActionBack:
				step = 2
			case ui.PromptActionExit:
				return exitOnboarding(stdout)
			case ui.PromptActionYes:
				if err := runOnboardingSetup(ctx, stdin, stdout, stderr, root, cfg); err != nil {
					return err
				}
				step = 4
			case ui.PromptActionNo:
				step = 4
			}
		case 4:
			printOnboardingStep(stdout, 4, 4, "Install", []ui.Section{{
				Rows: []ui.Row{
					{Key: "target", Value: target},
					{Key: "effect", Value: "copies the current honch executable into your user bin directory"},
				},
			}})
			action, err := prompts.ConfirmNavigate(fmt.Sprintf("Install honch to %s now?", target), false, true)
			if err != nil {
				return err
			}
			switch action {
			case ui.PromptActionBack:
				step = 3
			case ui.PromptActionExit:
				return exitOnboarding(stdout)
			case ui.PromptActionYes:
				if err := installOnboardingBinary(stdout, target); err != nil {
					return err
				}
				return completeOnboarding(stdout, root, cfg)
			case ui.PromptActionNo:
				return completeOnboarding(stdout, root, cfg)
			}
		}
	}
}

func printOnboardingStep(stdout io.Writer, current int, total int, name string, sections []ui.Section) {
	title := fmt.Sprintf("Step %d of %d: %s", current, total, name)
	_, _ = fmt.Fprint(stdout, ui.FormatSectionsWrapped(title, sections))
}

func exitOnboarding(stdout io.Writer) error {
	_, _ = fmt.Fprint(stdout, ui.FormatSections("Onboarding exited", []ui.Section{{
		Rows: []ui.Row{
			{Key: "resume", Value: "honch onboarding"},
		},
	}}))
	return errOnboardingExited
}

func completeOnboarding(stdout io.Writer, root string, cfg config.Config) error {
	if err := saveOnboardingState(root, cfg); err != nil {
		return err
	}
	_, _ = fmt.Fprint(stdout, ui.FormatSections("Honch onboarding complete", []ui.Section{{
		Rows: []ui.Row{
			{Key: "next", Value: "honch sandbox doctor"},
			{Key: "start", Value: "honch sandbox start"},
			{Key: "rerun", Value: "honch onboarding"},
		},
	}}))
	return nil
}

func onboardingSetupRows(report sandboxDoctorReport) []ui.Row {
	if report.Ready() {
		return []ui.Row{{Key: "status", Value: "ready"}}
	}
	return report.Missing
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

type siblingRepoSource struct {
	Name string
	URL  string
	Path string
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

func runOnboardingSetup(ctx context.Context, stdin io.Reader, stdout io.Writer, stderr io.Writer, root string, cfg config.Config) error {
	actions := setupActions(root, cfg)
	automated := make([]setupAction, 0, len(actions))
	manual := []ui.Row{}
	for _, action := range actions {
		if action.Run != nil {
			automated = append(automated, action)
			continue
		}
		manual = append(manual, ui.Row{Key: action.Name, Value: action.Command})
	}

	sections := []ui.Section{{Name: "actions", Rows: setupActionRows(actions)}}
	if len(manual) > 0 {
		sections = append(sections, ui.Section{Name: "manual", Rows: manual})
	}
	_, _ = fmt.Fprint(stdout, ui.FormatSections("Honch sandbox setup", sections))

	if len(automated) == 0 {
		return nil
	}
	if err := runSetupActions(ctx, stdin, stdout, stderr, automated); err != nil {
		return err
	}
	return nil
}

func installOnboardingBinary(stdout io.Writer, target string) error {
	installed, err := installExecutable(target)
	if err != nil {
		return err
	}
	if installed {
		_, _ = fmt.Fprintf(stdout, "Installed honch to %s\n", target)
	} else {
		_, _ = fmt.Fprintf(stdout, "honch is already installed at %s\n", target)
	}
	_, _ = fmt.Fprintln(stdout, "Reload your shell or run `hash -r` so the new binary is picked up from PATH.")
	return nil
}

func onboardingStatePath(root string, cfg config.Config) string {
	return filepath.Join(root, cfg.Sandbox.StateDir, "onboarding.json")
}

func onboardingComplete(root string, cfg config.Config) (bool, error) {
	path := onboardingStatePath(root, cfg)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	var state onboardingState
	if err := json.Unmarshal(data, &state); err != nil {
		return false, err
	}
	return state.Version >= onboardingStateVersion, nil
}

func saveOnboardingState(root string, cfg config.Config) error {
	path := onboardingStatePath(root, cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	state := onboardingState{Version: onboardingStateVersion, CompletedAt: time.Now().UTC()}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
