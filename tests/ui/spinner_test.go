package ui_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/honch/sdk/tools/sandbox/internal/ui"
)

func TestWithSpinnerPlainPrintsStartAndDone(t *testing.T) {
	ui.SetPlain(true)
	t.Cleanup(func() { ui.SetPlain(false) })
	var out bytes.Buffer

	err := ui.WithSpinner(context.Background(), bytes.NewBuffer(nil), &out, "building harness", func(context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("WithSpinner returned error: %v", err)
	}
	text := out.String()
	for _, want := range []string{"building harness", "done"} {
		if !strings.Contains(text, want) {
			t.Fatalf("spinner output missing %q:\n%s", want, text)
		}
	}
}
