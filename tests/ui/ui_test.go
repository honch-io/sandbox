package ui_test

import (
	"strings"
	"testing"

	"github.com/honch/sdk/tools/sandbox/internal/ui"
)

func TestFormatKeyValuesUsesIndentedArrowLayout(t *testing.T) {
	ui.SetPlain(false)
	out := ui.FormatKeyValues("Honch sandbox", []ui.Row{
		{Key: "session", Value: "inactive"},
		{Key: "clickhouse port", Value: 8123},
	})

	for _, want := range []string{
		"  Honch sandbox",
		"     session            ›   inactive",
		"     clickhouse port    ›   8123",
	} {
		if !strings.Contains(ui.StripANSI(out), want) {
			t.Fatalf("formatted output missing %q:\n%s", want, ui.StripANSI(out))
		}
	}
}

func TestFormatSectionsGroupsRows(t *testing.T) {
	ui.SetPlain(false)
	out := ui.FormatSections("Honch sandbox", []ui.Section{
		{
			Name: "session",
			Rows: []ui.Row{
				{Key: "session", Value: "inactive"},
			},
		},
		{
			Name: "ports",
			Rows: []ui.Row{
				{Key: "proxy port", Value: 18080},
			},
		},
	})

	plain := ui.StripANSI(out)
	for _, want := range []string{
		"     session\n",
		"       session            ›   inactive",
		"     ports\n",
		"       proxy port         ›   18080",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("section output missing %q:\n%s", want, plain)
		}
	}
}

func TestFormatErrorUsesRows(t *testing.T) {
	ui.SetPlain(false)
	out := ui.FormatError("missing event name", []ui.Row{
		{Key: "required", Value: "honch sandbox track <event>"},
		{Key: "example", Value: "honch sandbox track camera.motion"},
	})

	plain := ui.StripANSI(out)
	for _, want := range []string{
		"missing event name",
		"     required ›   honch sandbox track <event>",
		"     example  ›   honch sandbox track camera.motion",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("error output missing %q:\n%s", want, plain)
		}
	}
}

func TestFormatSectionsWrappedUsesTwoLineRowsForPathLikeEntries(t *testing.T) {
	ui.SetPlain(false)
	fullPath := "/usr/bin/git"
	out := ui.FormatSectionsWrapped("Honch sandbox", []ui.Section{
		{
			Name: "host",
			Rows: []ui.Row{
				{Key: "git", Value: fullPath},
				{Key: "worker", Value: "dirty"},
			},
		},
		{
			Name: "images",
			Rows: []ui.Row{
				{Key: "postgres:16-alpine", Value: "missing"},
			},
		},
	})
	plain := ui.StripANSI(out)

	for _, want := range []string{
		"git",
		fullPath,
		"\n        ›   ",
		"worker",
		"›   dirty",
		"postgres:16-alpine",
		"›   missing",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("wrapped output missing %q:\n%s", want, plain)
		}
	}
}

func TestSilentErrorIsDetectable(t *testing.T) {
	err := ui.NewSilentError("sandbox setup is incomplete")
	if !ui.IsSilentError(err) {
		t.Fatalf("silent error was not detectable: %T", err)
	}
	if err.Error() != "sandbox setup is incomplete" {
		t.Fatalf("silent error changed message: %q", err.Error())
	}
}

func TestSetPlainDisablesANSI(t *testing.T) {
	ui.SetPlain(true)
	t.Cleanup(func() { ui.SetPlain(false) })

	out := ui.FormatKeyValues("Honch sandbox", []ui.Row{{Key: "session", Value: "inactive"}})
	if out != ui.StripANSI(out) {
		t.Fatalf("plain output included ANSI: %q", out)
	}
}

func TestFormatCommandHelpShowsDescriptionsUnderCommands(t *testing.T) {
	ui.SetPlain(false)
	out := ui.FormatCommandHelp("honch sandbox", "Run the Honch SDK E2E sandbox", "", nil, []ui.CommandRow{
		{Name: "battery", Description: "Set the live harness battery level"},
	})

	plain := ui.StripANSI(out)
	for _, want := range []string{
		"  honch sandbox",
		"    Run the Honch SDK E2E sandbox",
		"    battery",
		"      Set the live harness battery level",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("help output missing %q:\n%s", want, plain)
		}
	}
}

func TestFormatGroupedCommandHelpUsesSectionsAndArrows(t *testing.T) {
	ui.SetPlain(false)
	out := ui.FormatGroupedCommandHelp(
		"honch sandbox",
		"Run the Honch SDK E2E sandbox",
		"start -> run c-core --detach -> track",
		[]ui.Section{
			{
				Name: "Global",
				Rows: []ui.Row{{Key: "--plain", Value: "disable styled output"}},
			},
			{
				Name: "Harness",
				Rows: []ui.Row{{Key: "run --detach", Value: "run harness in the background"}},
			},
		},
		[]ui.CommandSection{
			{
				Name: "Stack",
				Commands: []ui.CommandRow{
					{Name: "start", Description: "Start the local Honch stack"},
					{Name: "stop", Description: "Stop sandbox services"},
				},
			},
		},
	)

	plain := ui.StripANSI(out)
	for _, want := range []string{
		"  honch sandbox",
		"    Flow",
		"      start -> run c-core --detach -> track",
		"    Flags",
		"      Global",
		"--plain ›   disable styled output",
		"      Harness",
		"run --detach ›   run harness in the background",
		"    Stack",
		"      start  ›   Start the local Honch stack",
		"      stop   ›   Stop sandbox services",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("grouped help output missing %q:\n%s", want, plain)
		}
	}
}

func TestFormatCommandHelpDoesNotBoldDescriptions(t *testing.T) {
	ui.SetPlain(false)
	out := ui.FormatCommandHelp("honch sandbox", "Run the Honch SDK E2E sandbox", "", nil, []ui.CommandRow{
		{Name: "battery", Description: "Set the live harness battery level"},
	})

	if strings.Contains(out, "\x1b[1mSet the live harness battery level") {
		t.Fatalf("description was bolded:\n%q", out)
	}
}
