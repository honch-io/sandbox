package commands

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/scenario"
	"github.com/spf13/cobra"
)

func TestScenarioNetworkStepRequiresRunningSandbox(t *testing.T) {
	deps := Dependencies{RootDir: t.TempDir()}
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	sc := scenario.Scenario{Steps: []scenario.Step{
		{Network: &scenario.NetworkStep{Mode: "offline"}},
	}}

	err := runScenario(deps, cmd, sc)
	if err == nil {
		t.Fatal("scenario network step succeeded without a running sandbox")
	}
	if !strings.Contains(err.Error(), "sandbox is not running") {
		t.Fatalf("error did not explain inactive sandbox: %v", err)
	}
}

func TestScenarioWaitStopsWhenCommandContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	sc := scenario.Scenario{Steps: []scenario.Step{
		{Wait: &scenario.WaitStep{Duration: 200 * time.Millisecond}},
	}}

	started := time.Now()
	err := runScenario(Dependencies{RootDir: t.TempDir()}, cmd, sc)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runScenario error = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("runScenario waited %s after context cancellation", elapsed)
	}
}
