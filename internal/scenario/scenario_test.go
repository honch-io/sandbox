package scenario

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadParsesRepeatableScenarioSteps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "low-battery.yaml")
	if err := os.WriteFile(path, []byte(`
name: low battery reconnect
steps:
  - battery:
      level: 8
  - network:
      mode: offline
  - track:
      event: camera.motion
      properties:
        zone: porch
  - wait:
      duration: 250ms
  - flush: {}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	sc, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if sc.Name != "low battery reconnect" {
		t.Fatalf("name = %q", sc.Name)
	}
	if len(sc.Steps) != 5 {
		t.Fatalf("steps = %d", len(sc.Steps))
	}
	if sc.Steps[0].Battery.Level != 8 {
		t.Fatalf("battery level = %d", sc.Steps[0].Battery.Level)
	}
	if sc.Steps[3].Wait.Duration != 250*time.Millisecond {
		t.Fatalf("wait duration = %s", sc.Steps[3].Wait.Duration)
	}
}

func TestLoadRejectsStepWithoutAction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte("name: bad\nsteps:\n  - {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load accepted an empty step")
	}
}

func TestLoadRejectsNegativeWaitDuration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "negative-wait.yaml")
	if err := os.WriteFile(path, []byte(`
name: negative wait
steps:
  - wait:
      duration: -1s
`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load accepted a negative wait duration")
	}
	if !strings.Contains(err.Error(), "wait duration must be non-negative") {
		t.Fatalf("error did not explain negative wait: %v", err)
	}
}
