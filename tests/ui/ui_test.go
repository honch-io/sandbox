package ui_test

import (
	"strings"
	"testing"

	"honch.dev/honch/internal/ui"
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

func TestFormatSectionsTruncatesLongValuesToTerminalWidth(t *testing.T) {
	ui.SetPlain(false)
	t.Setenv("COLUMNS", "60")

	fullPath := "/Library/Frameworks/Python.framework/Versions/3.14/bin/python3"
	out := ui.FormatKeyValues("Honch sandbox", []ui.Row{
		{Key: "python", Value: fullPath},
	})
	plain := ui.StripANSI(out)

	if strings.Contains(plain, fullPath) {
		t.Fatalf("value was not truncated:\n%s", plain)
	}
	if !strings.Contains(plain, "...") || !strings.Contains(plain, "python3") {
		t.Fatalf("truncated value did not preserve a useful suffix:\n%s", plain)
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
