package commands

import (
	"bytes"
	"strings"
	"testing"

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
