package commands

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

type recordingTailClient struct {
	cancel context.CancelFunc
	since  []time.Time
}

func (c *recordingTailClient) Tail(ctx context.Context, cfg config.Config, since time.Time) (string, error) {
	c.since = append(c.since, since)
	if len(c.since) == 2 {
		c.cancel()
	}
	return "", nil
}

func TestTailEventsKeepsLookbackOverlapBetweenPolls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &recordingTailClient{cancel: cancel}

	err := tailEvents(ctx, io.Discard, config.Config{}, client, time.Unix(0, 0), time.Millisecond)
	if err != nil {
		t.Fatalf("tailEvents returned error: %v", err)
	}
	if len(client.since) != 2 {
		t.Fatalf("Tail calls = %d, want 2", len(client.since))
	}
	if age := time.Since(client.since[1]); age < eventTailLookback/2 {
		t.Fatalf("second tail cursor used too little lookback: age %s, want at least %s", age, eventTailLookback/2)
	}
}

type duplicateTailClient struct {
	cancel context.CancelFunc
	calls  int
}

func (c *duplicateTailClient) Tail(ctx context.Context, cfg config.Config, since time.Time) (string, error) {
	c.calls++
	if c.calls == 2 {
		c.cancel()
		return "event-1\nevent-2\n", nil
	}
	return "event-1\n", nil
}

func TestTailEventsSuppressesRowsAlreadySeenInLookback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &duplicateTailClient{cancel: cancel}
	var out bytes.Buffer

	err := tailEvents(ctx, &out, config.Config{}, client, time.Unix(0, 0), time.Millisecond)
	if err != nil {
		t.Fatalf("tailEvents returned error: %v", err)
	}
	if got := strings.Count(out.String(), "event-1"); got != 1 {
		t.Fatalf("event-1 printed %d times, want 1:\n%s", got, out.String())
	}
	if got := strings.Count(out.String(), "event-2"); got != 1 {
		t.Fatalf("event-2 printed %d times, want 1:\n%s", got, out.String())
	}
}

func TestTailSeenEvictsOldRows(t *testing.T) {
	seen := newTailSeen(2)
	if !seen.remember("event-1") {
		t.Fatal("first event-1 was not accepted")
	}
	if !seen.remember("event-2") {
		t.Fatal("event-2 was not accepted")
	}
	if !seen.remember("event-3") {
		t.Fatal("event-3 was not accepted")
	}
	if len(seen.keys) != 2 {
		t.Fatalf("seen keys = %d, want bounded size 2", len(seen.keys))
	}
	if !seen.remember("event-1") {
		t.Fatal("old event-1 was not evicted")
	}
}
