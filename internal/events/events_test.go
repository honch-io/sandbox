package events

import (
	"context"
	"errors"
	"net/http"
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

func TestListExplainsClickHouseConnectionFailure(t *testing.T) {
	cfg := config.Config{
		Ports: config.PortsConfig{ClickHouse: 8123},
		Sandbox: config.SandboxConfig{
			ClickHouseDatabase: "platform",
			ProjectID:          "00000000-0000-0000-0000-000000000002",
		},
	}
	client := Client{HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})}}

	_, err := client.List(context.Background(), cfg, 25)
	if err == nil {
		t.Fatal("List succeeded when ClickHouse request failed")
	}
	for _, want := range []string{"ClickHouse is not reachable", "honch sandbox start", "honch sandbox status"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q:\n%s", want, err.Error())
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
