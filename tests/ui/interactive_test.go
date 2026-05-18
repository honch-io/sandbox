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
