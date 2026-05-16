package commands

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/config"
	"github.com/honch/sdk/tools/sandbox/internal/session"
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
		"doctor",
		"setup",
		"images",
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
		"qemu",
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
		"start -> run esp-idf --detach -> track -> flush -> events list -> stop",
		"    Stack",
		"      start    ›   Start the local Honch stack",
		"    Harness",
		"      run      ›   Build and run an SDK harness",
		"      battery  ›   Set harness battery level",
		"    Setup",
		"      doctor   ›   Check local prerequisites",
		"      setup    ›   Install supported prerequisites",
		"      qemu     ›   Manage ESP-IDF QEMU tooling",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
}

func TestSandboxHelpDoesNotAdvertiseAdaptersAsCommands(t *testing.T) {
	root := NewRootCommand(Dependencies{})
	root.SetArgs([]string{"sandbox", "--help"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	help := ui.StripANSI(out.String())
	for _, misleading := range []string{"      c-core", "      esp-idf"} {
		if strings.Contains(help, misleading) {
			t.Fatalf("help advertised adapter as command %q:\n%s", misleading, help)
		}
	}
}

func TestLeafHelpShowsUsageAndFlags(t *testing.T) {
	root := NewRootCommand(Dependencies{})
	root.SetArgs([]string{"sandbox", "network", "--help"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	help := ui.StripANSI(out.String())
	for _, want := range []string{"Usage", "honch sandbox network --online|--offline|--server-error", "Flags", "--offline", "--online", "--server-error"} {
		if !strings.Contains(help, want) {
			t.Fatalf("leaf help missing %q:\n%s", want, help)
		}
	}
}

func TestNestedLeafHelpStaysOnRequestedCommand(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "images list", args: []string{"sandbox", "images", "list", "--help"}, want: "honch sandbox images list"},
		{name: "events list", args: []string{"sandbox", "events", "list", "--help"}, want: "honch sandbox events list"},
		{name: "qemu doctor", args: []string{"sandbox", "qemu", "doctor", "--help"}, want: "honch sandbox qemu doctor"},
		{name: "adapters list", args: []string{"sandbox", "adapters", "list", "--help"}, want: "honch sandbox adapters list"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := NewRootCommand(Dependencies{})
			root.SetArgs(append([]string{"--plain"}, tc.args...))
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)

			if err := root.Execute(); err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}
			help := ui.StripANSI(out.String())
			if !strings.Contains(help, tc.want) {
				t.Fatalf("nested leaf help missing %q:\n%s", tc.want, help)
			}
			if strings.Contains(help, "  honch sandbox\n") {
				t.Fatalf("nested leaf help fell back to sandbox help:\n%s", help)
			}
		})
	}
}

func TestUnknownNestedCommandsReturnErrors(t *testing.T) {
	for _, args := range [][]string{
		{"sandbox", "nope"},
		{"sandbox", "events", "nope"},
		{"sandbox", "qemu", "nope"},
	} {
		root := NewRootCommand(Dependencies{})
		root.SetArgs(append([]string{"--plain"}, args...))
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)

		err := root.Execute()
		if err == nil {
			t.Fatalf("%v succeeded; output:\n%s", args, out.String())
		}
		if !strings.Contains(err.Error(), "unknown command") {
			t.Fatalf("%v error did not explain unknown command: %v", args, err)
		}
	}
}

func TestSandboxSetupDryRunOffersSupportedInstallActions(t *testing.T) {
	rootDir := t.TempDir()
	binDir := filepath.Join(rootDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"git", "docker", "bun", "cargo", "cmake"} {
		path := filepath.Join(binDir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if runtime.GOOS == "darwin" {
		path := filepath.Join(binDir, "brew")
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", binDir)
	t.Setenv("IDF_PATH", "")
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "setup", "--dry-run"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("setup dry-run returned error: %v\n%s", err, out.String())
	}
	combined := out.String()
	for _, want := range []string{
		"Honch sandbox setup",
		"dry run",
		"honch sandbox qemu install",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("setup output missing %q:\n%s", want, combined)
		}
	}
	if runtime.GOOS == "darwin" && !strings.Contains(combined, "brew install python") {
		t.Fatalf("setup output missing brew Python install:\n%s", combined)
	}
}

func TestSandboxStartIsNoopWhenAlreadyRunning(t *testing.T) {
	rootDir := t.TempDir()
	manager := session.NewManager(filepath.Join(rootDir, ".honch-sandbox", "session.json"))
	if err := manager.Save(session.State{Stack: session.StackState{Running: true}}); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "start"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("start returned error: %v\n%s", err, out.String())
	}
	combined := out.String()
	if !strings.Contains(combined, "sandbox is already running") {
		t.Fatalf("start did not report existing sandbox:\n%s", combined)
	}
	if strings.Contains(combined, "Run platform database migrations") {
		t.Fatalf("start prompted for migrations even though sandbox was already running:\n%s", combined)
	}
}

func TestSandboxStopIsNoopWhenNotRunning(t *testing.T) {
	root := NewRootCommand(Dependencies{RootDir: t.TempDir()})
	root.SetArgs([]string{"--plain", "sandbox", "stop"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("stop returned error: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "sandbox is not running") {
		t.Fatalf("stop did not report inactive sandbox:\n%s", out.String())
	}
}

func TestSandboxStopClearsRunnerOnlySession(t *testing.T) {
	rootDir := t.TempDir()
	manager := session.NewManager(filepath.Join(rootDir, ".honch-sandbox", "session.json"))
	if err := manager.Save(session.State{Runner: session.RunnerState{Adapter: "c-core"}}); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "stop"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("stop returned error: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "sandbox has been stopped") {
		t.Fatalf("stop did not report stopped sandbox:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(rootDir, ".honch-sandbox", "session.json")); !os.IsNotExist(err) {
		t.Fatalf("stop did not clear runner-only session, stat err: %v", err)
	}
}

func TestSandboxStartRejectsConflictingMigrationFlags(t *testing.T) {
	root := NewRootCommand(Dependencies{RootDir: t.TempDir()})
	root.SetArgs([]string{"--plain", "sandbox", "start", "--migrate", "--skip-migrations"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("start accepted conflicting migration flags")
	}
	if !strings.Contains(err.Error(), "choose one migration mode") {
		t.Fatalf("start error did not explain migration flag conflict: %v", err)
	}
}

func TestSandboxStartHelpShowsMigrationFlags(t *testing.T) {
	root := NewRootCommand(Dependencies{})
	root.SetArgs([]string{"--plain", "sandbox", "start", "--help"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("help returned error: %v", err)
	}
	for _, want := range []string{"--migrate", "--skip-migrations"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("start help missing %q:\n%s", want, out.String())
		}
	}
}

func TestSandboxRunRejectsActiveRunner(t *testing.T) {
	rootDir := t.TempDir()
	manager := session.NewManager(filepath.Join(rootDir, ".honch-sandbox", "session.json"))
	if err := manager.Save(session.State{
		Stack:  session.StackState{Running: true},
		Runner: session.RunnerState{Adapter: "c-core", PID: os.Getpid(), Detached: true},
	}); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "run", "c-core", "--detach"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("run accepted a second active runner")
	}
	combined := err.Error() + "\n" + out.String()
	for _, want := range []string{"sandbox runner is already active", "c-core", "honch sandbox stop"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("run error missing %q:\n%s", want, combined)
		}
	}
}

func TestSandboxRunExplainsMissingAdapter(t *testing.T) {
	root := NewRootCommand(Dependencies{RootDir: t.TempDir()})
	root.SetArgs([]string{"--plain", "sandbox", "run"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("run succeeded without an adapter")
	}
	combined := err.Error() + "\n" + out.String()
	for _, want := range []string{"missing adapter name", "honch sandbox run c-core --detach", "honch sandbox adapters list"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("run error missing %q:\n%s", want, combined)
		}
	}
}

func TestBatteryExplainsMissingLevelBeforeSessionCheck(t *testing.T) {
	root := NewRootCommand(Dependencies{RootDir: t.TempDir()})
	root.SetArgs([]string{"--plain", "sandbox", "battery"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("battery succeeded without a level")
	}
	combined := err.Error() + "\n" + out.String()
	for _, want := range []string{"missing battery level", "honch sandbox battery --level <0-100>", "honch sandbox battery --level 8"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("battery error missing %q:\n%s", want, combined)
		}
	}
	if strings.Contains(combined, "session.json") {
		t.Fatalf("battery checked session before validating level:\n%s", combined)
	}
}

func TestNestedCommandsExplainMissingInputs(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "adapters show", args: []string{"sandbox", "adapters", "show"}, want: "honch sandbox adapters show c-core"},
		{name: "adapters doctor", args: []string{"sandbox", "adapters", "doctor"}, want: "honch sandbox adapters doctor c-core"},
		{name: "scenario run", args: []string{"sandbox", "scenario", "run"}, want: "honch sandbox scenario run <file.yaml>"},
		{name: "logs too many", args: []string{"sandbox", "logs", "device", "proxy"}, want: "honch sandbox logs [stack|device|proxy]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := NewRootCommand(Dependencies{RootDir: t.TempDir()})
			root.SetArgs(append([]string{"--plain"}, tc.args...))
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)

			err := root.Execute()
			if err == nil {
				t.Fatal("command succeeded without required input")
			}
			combined := err.Error() + "\n" + out.String()
			if !strings.Contains(combined, tc.want) {
				t.Fatalf("command error missing %q:\n%s", tc.want, combined)
			}
		})
	}
}

func TestLiveControlExplainsMissingSession(t *testing.T) {
	root := NewRootCommand(Dependencies{RootDir: t.TempDir()})
	root.SetArgs([]string{"--plain", "sandbox", "battery", "--level", "8"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("battery succeeded without an active session")
	}
	combined := err.Error() + "\n" + out.String()
	for _, want := range []string{"no active sandbox session", "honch sandbox start", "honch sandbox run c-core --detach"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("control error missing %q:\n%s", want, combined)
		}
	}
}

func TestLiveControlExplainsMissingRunner(t *testing.T) {
	rootDir := t.TempDir()
	manager := session.NewManager(filepath.Join(rootDir, ".honch-sandbox", "session.json"))
	if err := manager.Save(session.State{Stack: session.StackState{Running: true}}); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "flush"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("flush succeeded without an active runner")
	}
	combined := err.Error() + "\n" + out.String()
	for _, want := range []string{"no active sandbox runner", "honch sandbox run c-core --detach", "honch sandbox status"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("control error missing %q:\n%s", want, combined)
		}
	}
}

func TestNetworkRequiresRunningSandbox(t *testing.T) {
	root := NewRootCommand(Dependencies{RootDir: t.TempDir()})
	root.SetArgs([]string{"--plain", "sandbox", "network", "--offline"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("network succeeded without a running sandbox")
	}
	combined := err.Error() + "\n" + out.String()
	for _, want := range []string{"sandbox is not running", "honch sandbox start", "honch sandbox network --offline"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("network error missing %q:\n%s", want, combined)
		}
	}
}

func TestSandboxSetupDryRunOffersDockerImagePulls(t *testing.T) {
	rootDir := t.TempDir()
	binDir := filepath.Join(rootDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"git", "bun", "cargo", "cmake", "python"} {
		path := filepath.Join(binDir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	docker := filepath.Join(binDir, "docker")
	if err := os.WriteFile(docker, []byte("#!/bin/sh\nif [ \"$1 $2\" = \"image inspect\" ]; then exit 1; fi\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("IDF_PATH", rootDir)
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "setup", "--dry-run"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("setup dry-run returned error: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "honch sandbox images pull") {
		t.Fatalf("setup dry-run did not offer Docker image pulls:\n%s", out.String())
	}
}

func TestSandboxImagesListReportsImageStatus(t *testing.T) {
	rootDir := t.TempDir()
	binDir := filepath.Join(rootDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	docker := filepath.Join(binDir, "docker")
	script := "#!/bin/sh\nif [ \"$1 $2 $3\" = \"image inspect postgres:16-alpine\" ]; then exit 0; fi\nexit 1\n"
	if err := os.WriteFile(docker, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "images", "list"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("images list returned error: %v\n%s", err, out.String())
	}
	for _, want := range []string{"Honch sandbox images", "postgres:16-alpine", "present", "redis:7-alpine", "missing"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("images list missing %q:\n%s", want, out.String())
		}
	}
}

func TestSandboxImagesPullRunsDockerPulls(t *testing.T) {
	rootDir := t.TempDir()
	binDir := filepath.Join(rootDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(rootDir, "docker.log")
	docker := filepath.Join(binDir, "docker")
	script := "#!/bin/sh\necho \"$@\" >> " + logPath + "\nexit 0\n"
	if err := os.WriteFile(docker, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "images", "pull"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("images pull returned error: %v\n%s", err, out.String())
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"pull postgres:16-alpine",
		"pull redis:7-alpine",
		"pull clickhouse/clickhouse-server:24.8",
		"pull gcr.io/google.com/cloudsdktool/cloud-sdk:emulators",
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("docker pulls missing %q:\n%s", want, string(data))
		}
	}
}

func TestSandboxSetupRequiresConfirmationBeforeRunningActions(t *testing.T) {
	rootDir := t.TempDir()
	binDir := filepath.Join(rootDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"git", "docker", "bun", "cargo", "cmake"} {
		path := filepath.Join(binDir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 99\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if runtime.GOOS == "darwin" {
		path := filepath.Join(binDir, "brew")
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 99\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", binDir)
	t.Setenv("IDF_PATH", "")
	root := NewRootCommand(Dependencies{RootDir: rootDir, In: strings.NewReader("n\n")})
	root.SetArgs([]string{"--plain", "sandbox", "setup"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("setup succeeded after confirmation declined")
	}
	combined := err.Error() + "\n" + out.String()
	for _, want := range []string{
		"Run supported setup actions? [y/N]",
		"setup cancelled",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("setup output missing %q:\n%s", want, combined)
		}
	}
}

func TestSandboxDoctorReportsMissingPythonWithFix(t *testing.T) {
	rootDir := t.TempDir()
	binDir := filepath.Join(rootDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"git", "docker", "bun", "cargo", "cmake"} {
		path := filepath.Join(binDir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if runtime.GOOS == "darwin" {
		path := filepath.Join(binDir, "brew")
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", binDir)
	t.Setenv("IDF_PATH", "")
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "doctor"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("doctor succeeded with missing Python")
	}
	combined := err.Error() + "\n" + out.String()
	for _, want := range []string{
		"Honch sandbox doctor",
		"python",
		"install Python 3",
		"sandbox setup is incomplete",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, combined)
		}
	}
}

func TestSandboxDoctorReportsMissingDockerImages(t *testing.T) {
	rootDir := t.TempDir()
	binDir := filepath.Join(rootDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"git", "python", "bun", "cargo", "cmake", "idf.py", "qemu-system-xtensa"} {
		path := filepath.Join(binDir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	docker := filepath.Join(binDir, "docker")
	if err := os.WriteFile(docker, []byte("#!/bin/sh\nif [ \"$1 $2\" = \"image inspect\" ]; then exit 1; fi\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "darwin" {
		path := filepath.Join(binDir, "brew")
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", binDir)
	t.Setenv("IDF_PATH", rootDir)
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "doctor"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("doctor succeeded with missing Docker images")
	}
	combined := err.Error() + "\n" + out.String()
	for _, want := range []string{"images", "postgres:16-alpine", "missing", "honch sandbox images pull"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, combined)
		}
	}
}

func TestQEMUDoctorReportsMissingToolsWithInstallCommand(t *testing.T) {
	root := NewRootCommand(Dependencies{})
	root.SetArgs([]string{"--plain", "sandbox", "qemu", "doctor"})
	t.Setenv("PATH", t.TempDir())
	t.Setenv("IDF_PATH", "")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("doctor succeeded without ESP-IDF tools")
	}
	combined := err.Error() + "\n" + out.String()
	for _, want := range []string{
		"ESP-IDF QEMU tools are not ready",
		"idf.py",
		"qemu-system-xtensa",
		"IDF_PATH",
		"honch sandbox qemu install",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, combined)
		}
	}
}

func TestQEMUInstallUsesManagedPathAndRequiresConfirmation(t *testing.T) {
	rootDir := t.TempDir()
	root := NewRootCommand(Dependencies{RootDir: rootDir, In: strings.NewReader("n\n")})
	root.SetArgs([]string{"--plain", "sandbox", "qemu", "install"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("install succeeded without confirmation")
	}
	combined := err.Error() + "\n" + out.String()
	for _, want := range []string{
		"install cancelled",
		filepath.Join(rootDir, ".honch-sandbox", "toolchains", "esp-idf"),
		"Download and install ESP-IDF/QEMU tools? [y/N]",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("install output missing %q:\n%s", want, combined)
		}
	}
}

func TestQEMUInstallDryRunPrintsCommandsWithoutConfirmation(t *testing.T) {
	rootDir := t.TempDir()
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "qemu", "install", "--dry-run"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("dry-run returned error: %v", err)
	}
	for _, want := range []string{
		"dry run",
		"git clone --recursive --depth 1 --branch v6.0.1 https://github.com/espressif/esp-idf.git",
		"./install.sh esp32",
		"tools/idf_tools.py install qemu-xtensa qemu-riscv32",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out.String())
		}
	}
}

func TestQEMUDoctorRecognizesManagedToolchainWithoutIDFPath(t *testing.T) {
	rootDir := t.TempDir()
	idfPath := filepath.Join(rootDir, ".honch-sandbox", "toolchains", "esp-idf")
	toolsDir := filepath.Join(idfPath, "tools")
	qemuDir := filepath.Join(rootDir, "qemu-bin")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(qemuDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolsDir, "idf.py"), []byte("# fake idf.py\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	qemuPath := filepath.Join(qemuDir, "qemu-system-xtensa")
	if err := os.WriteFile(qemuPath, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	exportScript := "export PATH=" + qemuDir + ":$PATH\n"
	if err := os.WriteFile(filepath.Join(idfPath, "export.sh"), []byte(exportScript), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("IDF_PATH", "")

	status := qemuToolStatus(rootDir, configForTest())
	if !status.Ready() {
		t.Fatalf("managed toolchain was not ready: %+v", status)
	}
	if status.IDFSource != "managed" {
		t.Fatalf("IDFSource = %q, want managed", status.IDFSource)
	}
	if status.IDFPy != filepath.Join(toolsDir, "idf.py") {
		t.Fatalf("IDFPy = %q", status.IDFPy)
	}
	if status.QEMUXtensa != qemuPath {
		t.Fatalf("QEMUXtensa = %q, want %q", status.QEMUXtensa, qemuPath)
	}
}

func TestQEMUInstallPlanRunsManagedBootstrapCommands(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("shell script test requires /bin/sh")
	}
	rootDir := t.TempDir()
	binDir := filepath.Join(rootDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(rootDir, "install.log")
	writeFakeCommand := func(name string, body string) {
		t.Helper()
		path := filepath.Join(binDir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeFakeCommand("git", "printf 'git %s\\n' \"$*\" >> "+logPath+"\nfor idf_path do :; done\nmkdir -p \"$idf_path/tools\"\nprintf '#!/bin/sh\\nprintf install.sh\\\\n >> "+logPath+"\\n' > \"$idf_path/install.sh\"\nchmod +x \"$idf_path/install.sh\"\ntouch \"$idf_path/tools/idf_tools.py\"\n")
	writeFakeCommand("brew", "printf 'brew %s\\n' \"$*\" >> "+logPath+"\n")
	writeFakeCommand("python3", "printf 'python3 %s\\n' \"$*\" >> "+logPath+"\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var out bytes.Buffer
	idfPath := filepath.Join(rootDir, "managed-idf")
	err := runQEMUInstallPlan(context.Background(), &out, &out, qemuInstallPlanSpec{
		IDFPath: idfPath,
		Ref:     "v-test",
		Python:  "python3",
	})
	if err != nil {
		t.Fatalf("install plan returned error: %v\n%s", err, out.String())
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	for _, want := range []string{
		"git clone --recursive --depth 1 --branch v-test https://github.com/espressif/esp-idf.git " + idfPath,
		"install.sh",
		"python3 tools/idf_tools.py install qemu-xtensa qemu-riscv32",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("install log missing %q:\n%s", want, log)
		}
	}
}

func TestQEMUInstallPlanRejectsExistingNonIDFDirectory(t *testing.T) {
	idfPath := t.TempDir()
	var out bytes.Buffer
	err := runQEMUInstallPlan(context.Background(), &out, &out, qemuInstallPlanSpec{
		IDFPath: idfPath,
		Ref:     "v-test",
		Python:  "python3",
	})
	if err == nil {
		t.Fatal("install accepted an existing non-ESP-IDF directory")
	}
	for _, want := range []string{"existing path is not an ESP-IDF checkout", idfPath} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q:\n%s", want, err.Error())
		}
	}
}

func TestQEMUInstallPlanRejectsMissingPythonBeforeClone(t *testing.T) {
	rootDir := t.TempDir()
	binDir := filepath.Join(rootDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitPath := filepath.Join(binDir, "git")
	if err := os.WriteFile(gitPath, []byte("#!/bin/sh\nexit 42\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	var out bytes.Buffer
	err := runQEMUInstallPlan(context.Background(), &out, &out, qemuInstallPlanSpec{
		IDFPath: filepath.Join(rootDir, "managed-idf"),
		Ref:     "v-test",
		Python:  "",
	})
	if err == nil {
		t.Fatal("install accepted missing Python")
	}
	if !strings.Contains(err.Error(), "python is required") {
		t.Fatalf("error did not explain missing Python: %v", err)
	}
	if strings.Contains(out.String(), "git clone") {
		t.Fatalf("install cloned before Python preflight:\n%s", out.String())
	}
}

func TestQEMUInstallPlanRequiresHomebrewOnMacOS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Homebrew preflight is macOS-specific")
	}
	rootDir := t.TempDir()
	binDir := filepath.Join(rootDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"git", "python3"} {
		path := filepath.Join(binDir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", binDir)

	var out bytes.Buffer
	err := runQEMUInstallPlan(context.Background(), &out, &out, qemuInstallPlanSpec{
		IDFPath: filepath.Join(rootDir, "managed-idf"),
		Ref:     "v-test",
		Python:  "python3",
	})
	if err == nil {
		t.Fatal("install accepted missing Homebrew on macOS")
	}
	if !strings.Contains(err.Error(), "Homebrew is required") {
		t.Fatalf("error did not explain missing Homebrew: %v", err)
	}
	if strings.Contains(out.String(), "git clone") {
		t.Fatalf("install cloned before Homebrew preflight:\n%s", out.String())
	}
}

func TestRunEspIDFMissingToolsSuggestsManagedInstall(t *testing.T) {
	rootDir := t.TempDir()
	writeAdapterRegistryForTest(t, rootDir)
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "run", "esp-idf", "--detach"})
	t.Setenv("PATH", t.TempDir())
	t.Setenv("IDF_PATH", "")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("run esp-idf succeeded without tools")
	}
	for _, want := range []string{"ESP-IDF QEMU tools are not ready", "honch sandbox qemu install"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("run error missing %q:\n%s", want, err.Error())
		}
	}
}

func TestRunCommandRejectsUnknownAdapterWithRegistryNames(t *testing.T) {
	rootDir := t.TempDir()
	writeAdapterRegistryForTest(t, rootDir)
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "run", "micropython"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("run accepted unknown adapter")
	}
	for _, want := range []string{"unsupported adapter", "c-core", "esp-idf"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("run error missing %q:\n%s", want, err.Error())
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

func TestNetworkCommandRejectsInactiveSandbox(t *testing.T) {
	rootDir := t.TempDir()
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "network", "--online"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("network succeeded with inactive sandbox")
	}
	if !strings.Contains(err.Error(), "sandbox is not running") {
		t.Fatalf("network error did not explain inactive sandbox: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, ".honch-sandbox", "session.json")); !os.IsNotExist(err) {
		t.Fatalf("network command created inactive session, stat err: %v", err)
	}
}

func TestRunnerProxyStateShowsNotRunningWhenProxyPortClosed(t *testing.T) {
	cfg := configForTest()
	cfg.Ports.Proxy = unusedTCPPort(t)

	state := runnerProxyState(context.Background(), cfg)
	if state.Mode != "not running" {
		t.Fatalf("proxy mode = %q, want not running", state.Mode)
	}
	if state.Port != cfg.Ports.Proxy {
		t.Fatalf("proxy port = %d, want %d", state.Port, cfg.Ports.Proxy)
	}
}

func TestBatteryCommandPrintsControlConfirmation(t *testing.T) {
	rootDir := t.TempDir()
	controlPath := filepath.Join(rootDir, "control.jsonl")
	if err := os.WriteFile(controlPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	manager := session.NewManager(filepath.Join(rootDir, ".honch-sandbox", "session.json"))
	if err := manager.Save(session.State{
		Runner: session.RunnerState{Adapter: "c-core", ControlPath: controlPath},
	}); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "battery", "--level", "8"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("battery returned error: %v", err)
	}
	if !strings.Contains(out.String(), "battery control has been sent") {
		t.Fatalf("battery did not print confirmation:\n%s", out.String())
	}
	data, err := os.ReadFile(controlPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"action":"battery"`, `"level":8`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("control file missing %q:\n%s", want, string(data))
		}
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

func TestStartProxyProcessRejectsUnownedPortListener(t *testing.T) {
	rootDir := t.TempDir()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	cfg := configForTest()
	cfg.Ports.Proxy = listener.Addr().(*net.TCPAddr).Port

	proc, err := startProxyProcess(rootDir, cfg)
	if err == nil {
		t.Fatal("startProxyProcess accepted unowned proxy port listener")
	}
	if proc != nil {
		t.Fatalf("startProxyProcess returned process for unowned listener: %+v", proc)
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("error did not explain port ownership: %v", err)
	}
}

func TestWaitForRunnerReadyTimesOutWithoutReadyMarker(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "device.log")
	if err := os.WriteFile(logPath, []byte("booting\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := waitForRunnerReady(context.Background(), logPath, os.Getpid(), 20*time.Millisecond)
	if err == nil {
		t.Fatal("waitForRunnerReady succeeded without ready marker")
	}
	if !strings.Contains(err.Error(), "did not report ready") {
		t.Fatalf("error did not explain missing ready marker: %v", err)
	}
}

func configForTest() config.Config {
	return config.Config{
		Sandbox: config.SandboxConfig{StateDir: ".honch-sandbox"},
	}
}

func unusedTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func writeAdapterRegistryForTest(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, "tools", "sandbox", "adapters")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"c-core.yaml":  "name: c-core\nkind: posix\nharness: harnesses/c-core\nbuild:\n  tool: cmake\ncontrols:\n  transport: newline-json\n",
		"esp-idf.yaml": "name: esp-idf\nkind: qemu-esp32\nharness: harnesses/esp-idf\nbuild:\n  tool: idf.py\n  target: esp32\nemulator:\n  tool: qemu-system-xtensa\ncontrols:\n  transport: newline-json-uart\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
