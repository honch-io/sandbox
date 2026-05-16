package health

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

func HTTPStatus(ctx context.Context, url string, timeout time.Duration) string {
	return httpStatus(ctx, http.DefaultClient, url, timeout)
}

func httpStatus(ctx context.Context, client *http.Client, url string, timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "down: invalid URL"
	}
	resp, err := client.Do(req)
	if err != nil {
		return "down: " + conciseErr(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return "up"
	}
	return fmt.Sprintf("down: HTTP %d", resp.StatusCode)
}

func TCPStatus(ctx context.Context, address string, timeout time.Duration) string {
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return "down: " + conciseErr(err)
	}
	_ = conn.Close()
	return "up"
}

func ClickHouseStatus(ctx context.Context, address string, timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	url := fmt.Sprintf("http://%s/", address)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader("SELECT 1"))
	if err != nil {
		return "down: invalid URL"
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "down: " + conciseErr(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return "up"
	}
	return fmt.Sprintf("down: HTTP %d", resp.StatusCode)
}

func conciseErr(err error) string {
	text := err.Error()
	for _, marker := range []string{"connect: ", "dial tcp "} {
		if idx := strings.LastIndex(text, marker); idx >= 0 {
			return text[idx+len(marker):]
		}
	}
	return text
}
