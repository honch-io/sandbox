package ui_test

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"honch.dev/honch/internal/ui"
)

func TestProcessViewerModelAppendsOutputAndShowsCompletion(t *testing.T) {
	done := make(chan error, 1)
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	model := ui.NewProcessViewerModel("Honch sandbox run c-core", "waiting for harness output...\n", ui.ViewerFooter(), done, cancel, nil)

	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model = updated.(ui.ProcessViewerModel)

	updated, _ = model.Update(ui.AppendMsg{Text: "{\"ready\":true}\n"})
	model = updated.(ui.ProcessViewerModel)

	updated, _ = model.Update(ui.ProcessDoneMsg{Err: nil})
	model = updated.(ui.ProcessViewerModel)

	plain := ui.StripANSI(model.View())
	for _, want := range []string{
		"waiting for harness output...",
		"{\"ready\":true}",
		"run complete",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("process viewer output missing %q:\n%s", want, plain)
		}
	}
}

func TestProcessViewerModelShowsCancellation(t *testing.T) {
	done := make(chan error, 1)
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	model := ui.NewProcessViewerModel("Honch sandbox run c-core", "waiting for harness output...\n", ui.ViewerFooter(), done, cancel, nil)
	updated, _ := model.Update(ui.ProcessDoneMsg{Err: context.Canceled})
	model = updated.(ui.ProcessViewerModel)

	plain := ui.StripANSI(model.View())
	if !strings.Contains(plain, "run cancelled") {
		t.Fatalf("process viewer did not show cancellation:\n%s", plain)
	}
}

func TestProcessViewerModelShortcutPrefillsBatteryCommand(t *testing.T) {
	done := make(chan error, 1)
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	model := ui.NewProcessViewerModel("Honch sandbox run c-core", "waiting for harness output...\n", ui.ViewerFooter(), done, cancel, nil)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	model = updated.(ui.ProcessViewerModel)

	plain := ui.StripANSI(model.View())
	for _, want := range []string{"command", "battery 4", "▌", "press enter to send battery 4"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("shortcut view missing %q:\n%s", want, plain)
		}
	}
}

func TestProcessViewerModelShortcutPrefillsTrackCommand(t *testing.T) {
	done := make(chan error, 1)
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	model := ui.NewProcessViewerModel("Honch sandbox run c-core", "waiting for harness output...\n", ui.ViewerFooter(), done, cancel, nil)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	model = updated.(ui.ProcessViewerModel)

	plain := ui.StripANSI(model.View())
	for _, want := range []string{"track camera.motion", "example=", "add a value after example="} {
		if !strings.Contains(plain, want) {
			t.Fatalf("shortcut view missing %q:\n%s", want, plain)
		}
	}
}

func TestProcessViewerModelTypedCommandSubmits(t *testing.T) {
	done := make(chan error, 1)
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var submitted string
	model := ui.NewProcessViewerModel("Honch sandbox run c-core", "waiting for harness output...\n", ui.ViewerFooter(), done, cancel, func(raw string) (string, error) {
		submitted = raw
		return "sent: " + raw, nil
	})

	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune(":")},
		{Type: tea.KeyRunes, Runes: []rune("f")},
		{Type: tea.KeyRunes, Runes: []rune("l")},
		{Type: tea.KeyRunes, Runes: []rune("u")},
		{Type: tea.KeyRunes, Runes: []rune("s")},
		{Type: tea.KeyRunes, Runes: []rune("h")},
		{Type: tea.KeyEnter},
	} {
		updated, _ := model.Update(key)
		model = updated.(ui.ProcessViewerModel)
	}

	if submitted != "flush" {
		t.Fatalf("submitted command = %q, want flush", submitted)
	}
	plain := ui.StripANSI(model.View())
	if !strings.Contains(plain, "sent: flush") {
		t.Fatalf("process viewer did not show submission status:\n%s", plain)
	}
}

func TestProcessViewerModelTabSwitchesToEventsPane(t *testing.T) {
	done := make(chan error, 1)
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	model := ui.NewProcessViewerModel("Honch sandbox run c-core", "waiting for harness output...\n", ui.ViewerFooter(), done, cancel, nil)
	for _, msg := range []tea.Msg{
		ui.AppendMsg{Text: "run output\n"},
		ui.EventAppendMsg{Text: "event output\n"},
		tea.KeyMsg{Type: tea.KeyTab},
	} {
		updated, _ := model.Update(msg)
		model = updated.(ui.ProcessViewerModel)
	}

	plain := ui.StripANSI(model.View())
	for _, want := range []string{"events tail", "event output"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("tab view missing %q:\n%s", want, plain)
		}
	}
}

func TestProcessViewerModelShowsEmptyEventsPaneCopy(t *testing.T) {
	done := make(chan error, 1)
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	model := ui.NewProcessViewerModel("Honch sandbox run c-core", "waiting for harness output...\n", ui.ViewerFooter(), done, cancel, nil)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(ui.ProcessViewerModel)

	plain := ui.StripANSI(model.View())
	if !strings.Contains(plain, "No events yet...") {
		t.Fatalf("empty events view missing copy:\n%s", plain)
	}
}

func TestProcessViewerModelWithEventsContentRendersHistory(t *testing.T) {
	done := make(chan error, 1)
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	model := ui.NewProcessViewerModel("Honch sandbox run c-core", "waiting for harness output...\n", ui.ViewerFooter(), done, cancel, nil)
	model = model.WithEventsContent("event output\n")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(ui.ProcessViewerModel)

	plain := ui.StripANSI(model.View())
	for _, want := range []string{"event output", "tab run"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("seeded events view missing %q:\n%s", want, plain)
		}
	}
}

func TestParseProcessCommand(t *testing.T) {
	action, fields, status, err := ui.ParseProcessCommand(`track camera.motion {"zone":"porch"}`)
	if err != nil {
		t.Fatalf("ParseProcessCommand returned error: %v", err)
	}
	if action != "track" {
		t.Fatalf("action = %q, want track", action)
	}
	if fields["event"] != "camera.motion" {
		t.Fatalf("event = %#v, want camera.motion", fields["event"])
	}
	if status != "track control has been sent: camera.motion" {
		t.Fatalf("status = %q", status)
	}
}
