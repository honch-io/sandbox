package main

import (
	"context"
	"errors"
	"testing"
)

func TestMainExitCodeTreatsCancellationAsSuccess(t *testing.T) {
	if got := mainExitCode(nil); got != 0 {
		t.Fatalf("mainExitCode(nil) = %d, want 0", got)
	}
	if got := mainExitCode(context.Canceled); got != 0 {
		t.Fatalf("mainExitCode(context.Canceled) = %d, want 0", got)
	}
	if got := mainExitCode(errors.New("boom")); got != 1 {
		t.Fatalf("mainExitCode(error) = %d, want 1", got)
	}
}
