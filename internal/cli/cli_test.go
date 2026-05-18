package cli

import (
	"context"
	"errors"
	"testing"
)

func TestMainExitCodeTreatsCancellationAsSuccess(t *testing.T) {
	if got := MainExitCode(nil); got != 0 {
		t.Fatalf("MainExitCode(nil) = %d, want 0", got)
	}
	if got := MainExitCode(context.Canceled); got != 0 {
		t.Fatalf("MainExitCode(context.Canceled) = %d, want 0", got)
	}
	if got := MainExitCode(errors.New("boom")); got != 1 {
		t.Fatalf("MainExitCode(error) = %d, want 1", got)
	}
}
