package commands

import (
	"sync"
	"testing"

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
