package events

import (
	"strings"
	"testing"

	"github.com/honch/sdk/tools/sandbox/internal/config"
)

func TestListQueryFiltersByTeamID(t *testing.T) {
	cfg := config.Config{
		Sandbox: config.SandboxConfig{
			ClickHouseDatabase: "platform",
			ProjectID:          "00000000-0000-0000-0000-000000000002",
		},
	}

	query := ListQuery(cfg, 25)
	if !strings.Contains(query, "WHERE team_id = '00000000-0000-0000-0000-000000000002'") {
		t.Fatalf("query does not filter by team_id:\n%s", query)
	}
	if strings.Contains(query, "project_id") {
		t.Fatalf("query should not reference project_id:\n%s", query)
	}
}
