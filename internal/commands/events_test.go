package commands

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/config"
)

type fakeTailClient struct {
	cancel context.CancelFunc
	calls  int
}

func (c *fakeTailClient) Tail(ctx context.Context, cfg config.Config, since time.Time) (string, error) {
	c.calls++
	if c.calls == 2 {
		c.cancel()
	}
	return fmt.Sprintf("batch-%d\n", c.calls), nil
}

func TestTailEventsPollsUntilContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &fakeTailClient{cancel: cancel}
	var out bytes.Buffer

	err := tailEvents(ctx, &out, config.Config{}, client, time.Unix(0, 0), time.Millisecond)
	if err != nil {
		t.Fatalf("tailEvents returned error: %v", err)
	}
	if client.calls != 2 {
		t.Fatalf("Tail calls = %d, want 2", client.calls)
	}
	for _, want := range []string{"batch-1", "batch-2"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("tail output missing %q:\n%s", want, out.String())
		}
	}
}
