package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestWithSpinnerPlainPrintsStartAndDone(t *testing.T) {
	SetPlain(true)
	t.Cleanup(func() { SetPlain(false) })
	var out bytes.Buffer

	err := WithSpinner(context.Background(), &out, "building harness", func() error {
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
