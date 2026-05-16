package ui

import (
	"strings"
	"testing"
)

func TestFormatKeyValuesUsesIndentedArrowLayout(t *testing.T) {
	SetPlain(false)
	out := FormatKeyValues("Honch sandbox", []Row{
		{Key: "session", Value: "inactive"},
		{Key: "clickhouse port", Value: 8123},
	})

	for _, want := range []string{
		"  Honch sandbox",
		"     session            ›   inactive",
		"     clickhouse port    ›   8123",
	} {
		if !strings.Contains(StripANSI(out), want) {
			t.Fatalf("formatted output missing %q:\n%s", want, StripANSI(out))
		}
	}
}

func TestFormatSectionsGroupsRows(t *testing.T) {
	SetPlain(false)
	out := FormatSections("Honch sandbox", []Section{
		{
			Name: "session",
			Rows: []Row{
				{Key: "session", Value: "inactive"},
			},
		},
		{
			Name: "ports",
			Rows: []Row{
				{Key: "proxy port", Value: 18080},
			},
		},
	})

	plain := StripANSI(out)
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
	SetPlain(false)
	out := FormatError("missing event name", []Row{
		{Key: "required", Value: "honch sandbox track <event>"},
		{Key: "example", Value: "honch sandbox track camera.motion"},
	})

	plain := StripANSI(out)
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

func TestSetPlainDisablesANSI(t *testing.T) {
	SetPlain(true)
	t.Cleanup(func() { SetPlain(false) })

	out := FormatKeyValues("Honch sandbox", []Row{{Key: "session", Value: "inactive"}})
	if out != StripANSI(out) {
		t.Fatalf("plain output included ANSI: %q", out)
	}
}

func TestFormatCommandHelpShowsDescriptionsUnderCommands(t *testing.T) {
	SetPlain(false)
	out := FormatCommandHelp("honch sandbox", "Run the Honch SDK E2E sandbox", "", nil, []CommandRow{
		{Name: "battery", Description: "Set the live harness battery level"},
	})

	plain := StripANSI(out)
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
	SetPlain(false)
	out := FormatGroupedCommandHelp(
		"honch sandbox",
		"Run the Honch SDK E2E sandbox",
		"start -> run c-core --detach -> track",
		[]CommandSection{
			{
				Name: "Stack",
				Commands: []CommandRow{
					{Name: "start", Description: "Start the local Honch stack"},
					{Name: "stop", Description: "Stop sandbox services"},
				},
			},
		},
	)

	plain := StripANSI(out)
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
	SetPlain(false)
	out := FormatCommandHelp("honch sandbox", "Run the Honch SDK E2E sandbox", "", nil, []CommandRow{
		{Name: "battery", Description: "Set the live harness battery level"},
	})

	if strings.Contains(out, "\x1b[1mSet the live harness battery level") {
		t.Fatalf("description was bolded:\n%q", out)
	}
}
