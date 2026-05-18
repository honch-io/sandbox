package events

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/config"
)

func TestListQueryFiltersByTeamID(t *testing.T) {
	cfg := config.Config{
		Sandbox: config.SandboxConfig{
			ClickHouseDatabase: "platform",
			ProjectID:          "00000000-0000-0000-0000-000000000002",
		},
	}

	query, err := ListQuery(cfg, 25)
	if err != nil {
		t.Fatalf("ListQuery returned error: %v", err)
	}
	if !strings.Contains(query, "WHERE team_id = '00000000-0000-0000-0000-000000000002'") {
		t.Fatalf("query does not filter by team_id:\n%s", query)
	}
	if strings.Contains(query, "project_id") {
		t.Fatalf("query should not reference project_id:\n%s", query)
	}
}

func TestTailQueryUsesLineOrientedFormat(t *testing.T) {
	cfg := config.Config{
		Sandbox: config.SandboxConfig{
			ClickHouseDatabase: "platform",
			ProjectID:          "00000000-0000-0000-0000-000000000002",
		},
	}

	query, err := TailQuery(cfg, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("TailQuery returned error: %v", err)
	}
	if !strings.Contains(query, "FORMAT JSONEachRow") {
		t.Fatalf("tail query is not line-oriented:\n%s", query)
	}
	if strings.Contains(query, "FORMAT PrettyCompact") {
		t.Fatalf("tail query uses table output that cannot be row de-duped:\n%s", query)
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

func TestListRejectsInvalidClickHouseDatabase(t *testing.T) {
	cfg := config.Config{
		Sandbox: config.SandboxConfig{
			ClickHouseDatabase: "platform; DROP TABLE events",
			ProjectID:          "00000000-0000-0000-0000-000000000002",
		},
	}
	called := false
	client := Client{HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("unexpected request")
	})}}

	_, err := client.List(context.Background(), cfg, 25)
	if err == nil {
		t.Fatal("List accepted an invalid ClickHouse database identifier")
	}
	if !strings.Contains(err.Error(), "invalid ClickHouse database") {
		t.Fatalf("error did not explain invalid database: %v", err)
	}
	if called {
		t.Fatal("List sent an HTTP request after invalid database validation failed")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
