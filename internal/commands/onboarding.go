package commands

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"honch.dev/honch/internal/config"
	"honch.dev/honch/internal/ui"
)

var errOnboardingExited = errors.New("onboarding exited")

var onboardingGate = defaultOnboardingGate
var cloneSiblingRepo = runSiblingRepoClone

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
