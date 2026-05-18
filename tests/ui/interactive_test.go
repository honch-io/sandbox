package ui_test

import (
	"bytes"
	"testing"

	"github.com/honch/sdk/tools/sandbox/internal/ui"
)

func TestPromptSessionUsesOneBufferedInputStream(t *testing.T) {
	ui.SetPlain(true)
	t.Cleanup(func() { ui.SetPlain(false) })

	in := bytes.NewBufferString("y\n\ncustom\n")
	var out bytes.Buffer
	prompts := ui.NewPromptSession(in, &out)

	confirmed, err := prompts.Confirm("Clone missing Honch repos now?")
	if err != nil {
		t.Fatalf("confirm returned error: %v", err)
	}
	if !confirmed {
		t.Fatal("confirm returned false, want true")
	}

	defaulted, err := prompts.Text("Clone destination parent", "/tmp/honch")
	if err != nil {
		t.Fatalf("defaulted text returned error: %v", err)
	}
	if defaulted != "/tmp/honch" {
		t.Fatalf("defaulted text = %q, want %q", defaulted, "/tmp/honch")
	}

	value, err := prompts.Text("Set capture repo path", "")
	if err != nil {
		t.Fatalf("text returned error: %v", err)
	}
	if value != "custom" {
		t.Fatalf("text = %q, want custom", value)
	}
}

func TestPromptSessionNavigationActionsUsePlainFallback(t *testing.T) {
	ui.SetPlain(true)
	t.Cleanup(func() { ui.SetPlain(false) })

	in := bytes.NewBufferString("b\nq\n")
	var out bytes.Buffer
	prompts := ui.NewPromptSession(in, &out)

	action, err := prompts.ConfirmNavigate("Run setup now?", false, true)
	if err != nil {
		t.Fatalf("ConfirmNavigate returned error: %v", err)
	}
	if action != ui.PromptActionBack {
		t.Fatalf("ConfirmNavigate action = %v, want back", action)
	}

	action, err = prompts.ContinueOrExit("Continue onboarding?")
	if err != nil {
		t.Fatalf("ContinueOrExit returned error: %v", err)
	}
	if action != ui.PromptActionExit {
		t.Fatalf("ContinueOrExit action = %v, want exit", action)
	}
}
