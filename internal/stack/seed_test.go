package stack

import (
	"strings"
	"testing"

	"github.com/honch/sdk/tools/sandbox/internal/config"
)

func TestSandboxSeedSQLCreatesOrganizationAndProject(t *testing.T) {
	cfg := config.Config{
		Sandbox: config.SandboxConfig{
			ProjectID: "00000000-0000-0000-0000-000000000002",
			Token:     "honch_e2e_test_key",
		},
	}

	sql := SandboxSeedSQL(cfg)
	for _, want := range []string{
		"INSERT INTO organizations",
		"INSERT INTO projects",
		"honch_e2e_test_key",
		"00000000-0000-0000-0000-000000000002",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("seed SQL missing %q:\n%s", want, sql)
		}
	}
}

func TestPostgresPrerequisiteSQLCreatesPgcrypto(t *testing.T) {
	sql := PostgresPrerequisiteSQL()
	if !strings.Contains(sql, "CREATE EXTENSION IF NOT EXISTS pgcrypto") {
		t.Fatalf("prerequisite SQL does not create pgcrypto:\n%s", sql)
	}
}
