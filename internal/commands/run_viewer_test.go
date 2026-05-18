package commands

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/config"
	"github.com/honch/sdk/tools/sandbox/internal/ui"
)

func TestProcessCommandExecutorAutoFlushesTrack(t *testing.T) {
	orig := sendHarnessControlFn
	t.Cleanup(func() { sendHarnessControlFn = orig })

	var mu sync.Mutex
	var calls []string
	sendHarnessControlFn = func(_ string, action string, fields map[string]any) error {
		mu.Lock()
		defer mu.Unlock()
		switch action {
		case "track":
			if fields["event"] != "camera.motion" {
				t.Fatalf("track event = %#v, want camera.motion", fields["event"])
			}
		case "flush":
		default:
			t.Fatalf("unexpected action %q", action)
		}
		calls = append(calls, action)
		return nil
	}

	exec := newProcessCommandExecutor("/tmp/control")
	status, err := exec(`track camera.motion zone=porch`)
	if err != nil {
		t.Fatalf("executor returned error: %v", err)
	}
	if status != "track control has been sent: camera.motion" {
		t.Fatalf("status = %q", status)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 2 || calls[0] != "track" || calls[1] != "flush" {
		t.Fatalf("calls = %#v, want [track flush]", calls)
	}
}

func TestProcessCommandExecutorRejectsInvalidCommands(t *testing.T) {
	_, _, _, err := ui.ParseProcessCommand("battery nope")
	if err == nil {
		t.Fatal("expected invalid battery command to fail")
	}
}

func TestTailLiveEventsUsesProvidedCursor(t *testing.T) {
	orig := tailEventsFn
	t.Cleanup(func() { tailEventsFn = orig })

	wantSince := time.Unix(123, 0).UTC()
	called := false
	tailEventsFn = func(
		ctx context.Context,
		in io.Reader,
		out io.Writer,
		cfg config.Config,
		client eventTailClient,
		since time.Time,
		interval time.Duration,
	) error {
		called = true
		if !since.Equal(wantSince) {
			t.Fatalf("since = %s, want %s", since, wantSince)
		}
		if interval != 2*time.Second {
			t.Fatalf("interval = %s, want 2s", interval)
		}
		return nil
	}

	if err := tailLiveEvents(context.Background(), config.Config{}, io.Discard, wantSince); err != nil {
		t.Fatalf("tailLiveEvents returned error: %v", err)
	}
	if !called {
		t.Fatal("tailEventsFn was not called")
	}
}
