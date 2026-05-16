package events

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/config"
)

type Client struct {
	HTTP *http.Client
}

func (c Client) List(ctx context.Context, cfg config.Config, limit int) (string, error) {
	return c.query(ctx, cfg, ListQuery(cfg, limit))
}

func (c Client) Tail(ctx context.Context, cfg config.Config, since time.Time) (string, error) {
	query := fmt.Sprintf(`SELECT event, timestamp, distinct_id FROM %s.events WHERE team_id = '%s' AND timestamp > '%s' ORDER BY timestamp ASC FORMAT PrettyCompact`,
		cfg.Sandbox.ClickHouseDatabase,
		strings.ReplaceAll(cfg.Sandbox.ProjectID, "'", "''"),
		since.UTC().Format("2006-01-02 15:04:05"),
	)
	return c.query(ctx, cfg, query)
}

func ListQuery(cfg config.Config, limit int) string {
	if limit <= 0 {
		limit = 25
	}
	return fmt.Sprintf(`SELECT event, timestamp, distinct_id FROM %s.events WHERE team_id = '%s' ORDER BY timestamp DESC LIMIT %d FORMAT PrettyCompact`,
		cfg.Sandbox.ClickHouseDatabase,
		strings.ReplaceAll(cfg.Sandbox.ProjectID, "'", "''"),
		limit,
	)
}

func (c Client) query(ctx context.Context, cfg config.Config, query string) (string, error) {
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	endpoint := url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", cfg.Ports.ClickHouse), Path: "/"}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader(query))
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ClickHouse is not reachable on 127.0.0.1:%d: %w\nstart the stack with `honch sandbox start`, then check health with `honch sandbox status`", cfg.Ports.ClickHouse, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("clickhouse returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return string(body), nil
}
