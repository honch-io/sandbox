package commands

import (
	"context"
	"fmt"
	"io"

	"honch.dev/honch/internal/config"
	"honch.dev/honch/internal/ui"
)

func onboardingSetupRows(report sandboxDoctorReport) []ui.Row {
	if report.Ready() {
		return []ui.Row{{Key: "status", Value: "ready"}}
	}
	return report.Missing
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
