package commands

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/ui"
)

func TestRootCommandExposesSandboxContract(t *testing.T) {
	root := NewRootCommand(Dependencies{})
	root.SetArgs([]string{"sandbox", "--help"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	help := out.String()
	for _, want := range []string{
		"start",
		"stop",
		"status",
		"update",
		"run",
		"battery",
		"network",
		"track",
		"flush",
		"reset",
		"logs",
		"events",
		"scenario",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
}

func TestRootHelpUsesSandboxHelpFormat(t *testing.T) {
	root := NewRootCommand(Dependencies{})
	root.SetArgs([]string{"sandbox", "--help"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	help := out.String()
	for _, want := range []string{
		"  honch sandbox",
		"    Flow",
		"    Stack",
		"      start    ›   Start the local Honch stack",
		"    Harness",
		"      battery  ›   Set harness battery level",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
}

func TestRootHelpHidesGeneratedHelpAndCompletion(t *testing.T) {
	root := NewRootCommand(Dependencies{})
	root.SetArgs([]string{"--help"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	help := out.String()
	for _, hidden := range []string{"completion", "Help about any command"} {
		if strings.Contains(help, hidden) {
			t.Fatalf("help included generated command %q:\n%s", hidden, help)
		}
	}
	for _, want := range []string{
		"  honch",
		"    Tools",
		"      sandbox ›   Run the Honch SDK E2E sandbox",
	} {
		if !strings.Contains(ui.StripANSI(help), want) {
			t.Fatalf("help missing %q:\n%s", want, ui.StripANSI(help))
		}
	}
}

func TestNetworkCommandRequiresExactlyOneMode(t *testing.T) {
	root := NewRootCommand(Dependencies{})
	root.SetArgs([]string{"sandbox", "network", "--online", "--offline"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("network accepted conflicting modes")
	}
	for _, want := range []string{"choose one network mode", "honch sandbox network --offline"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("network error missing %q:\n%s", want, err.Error())
		}
	}
}

func TestNetworkCommandDoesNotCreateSessionWhenInactive(t *testing.T) {
	rootDir := t.TempDir()
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"sandbox", "network", "--online"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, ".honch-sandbox", "session.json")); !os.IsNotExist(err) {
		t.Fatalf("network command created inactive session, stat err: %v", err)
	}
}

func TestTrackCommandExplainsRequiredEventArgument(t *testing.T) {
	root := NewRootCommand(Dependencies{})
	root.SetArgs([]string{"sandbox", "track"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("track accepted missing event")
	}
	for _, want := range []string{"missing event name", "honch sandbox track camera.motion"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("track error missing %q:\n%s", want, err.Error())
		}
	}
}

func TestParsePropertiesRequiresJSONObject(t *testing.T) {
	props, err := parseProperties(`{"zone":"porch"}`)
	if err != nil {
		t.Fatalf("parseProperties returned error: %v", err)
	}
	if props["zone"] != "porch" {
		t.Fatalf("zone property = %v, want porch", props["zone"])
	}

	if _, err := parseProperties(`["not", "an", "object"]`); err == nil {
		t.Fatal("parseProperties accepted a JSON array")
	}
}

func TestConfirmRequiresExplicitYes(t *testing.T) {
	var out bytes.Buffer
	ok, err := confirm(strings.NewReader("n\n"), &out, "Run migrations? ")
	if err != nil {
		t.Fatalf("confirm returned error: %v", err)
	}
	if ok {
		t.Fatal("confirm accepted no")
	}

	ok, err = confirm(strings.NewReader("yes\n"), &out, "Run migrations? ")
	if err != nil {
		t.Fatalf("confirm returned error: %v", err)
	}
	if !ok {
		t.Fatal("confirm rejected yes")
	}
}

func TestLogsCommandPrintsRecentLogContent(t *testing.T) {
	rootDir := t.TempDir()
	logDir := filepath.Join(rootDir, ".honch-sandbox", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "device.log"), []byte("device ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"sandbox", "logs", "device"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out.String(), "device ready") {
		t.Fatalf("logs output missing file content:\n%s", out.String())
	}
}

func TestPortIsOpenDetectsListeningProxyPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	if !portIsOpen(context.Background(), port, time.Second) {
		t.Fatal("portIsOpen returned false for listening port")
	}
}
