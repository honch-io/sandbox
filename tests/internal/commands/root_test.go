package commands

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/honch/sdk/tools/sandbox/internal/config"
	"github.com/honch/sdk/tools/sandbox/internal/session"
	"github.com/honch/sdk/tools/sandbox/internal/ui"
)

func assertContainsInOrder(t *testing.T, text string, wants []string) {
	t.Helper()
	start := 0
	for _, want := range wants {
		index := strings.Index(text[start:], want)
		if index < 0 {
			t.Fatalf("output missing %q after byte %d:\n%s", want, start, text)
		}
		start += index + len(want)
	}
}

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
		"config",
		"images",
		"update",
		"run",
		"battery",
		"network",
		"track",
		"flush",
		"reset",
		"logs",
		"flags",
		"events",
		"scenario",
		"qemu",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
}

func TestLoadRuntimeFindsRepoRootFromNestedDirectory(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.com/test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(repoRoot, "adapters"),
		filepath.Join(repoRoot, "harnesses", "c-core"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("test repo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(repoRoot, "work", "tree", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}

	root, _, _, err := loadRuntime(Dependencies{})
	if err != nil {
		t.Fatalf("loadRuntime returned error: %v", err)
	}
	if resolvedPath(root) != resolvedPath(repoRoot) {
		t.Fatalf("loadRuntime root = %q, want %q", root, repoRoot)
	}
}

func TestLoadRuntimeUsesInstalledSandboxRootWhenCurrentDirectoryIsOutsideCheckout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	installedRoot := filepath.Join(home, ".local", "share", "honch", "sandbox")
	if err := os.MkdirAll(installedRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installedRoot, "go.mod"), []byte("module example.com/test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(installedRoot, "adapters"),
		filepath.Join(installedRoot, "harnesses"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
	outside := t.TempDir()
	if err := os.Chdir(outside); err != nil {
		t.Fatal(err)
	}

	root, _, _, err := loadRuntime(Dependencies{})
	if err != nil {
		t.Fatalf("loadRuntime returned error: %v", err)
	}
	if resolvedPath(root) != resolvedPath(installedRoot) {
		t.Fatalf("loadRuntime root = %q, want installed root %q", root, installedRoot)
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
		"      config   ›   List and edit local config",
		"      qemu     ›   Manage ESP-IDF QEMU tooling",
		"    Inspect",
		"      flags    ›   Inspect command flags",
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
		{name: "config list", args: []string{"sandbox", "config", "list", "--help"}, want: "honch sandbox config list"},
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

func TestSandboxStatusMarksProxyInactiveWithoutSession(t *testing.T) {
	root := NewRootCommand(Dependencies{RootDir: t.TempDir()})
	root.SetArgs([]string{"--plain", "sandbox", "status"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("status returned error: %v\n%s", err, out.String())
	}
	combined := out.String()
	for _, want := range []string{"proxy health", "down: inactive sandbox"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("status did not report inactive proxy health %q:\n%s", want, combined)
		}
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

func TestSandboxStartRollsBackStackWhenProxyStartupFails(t *testing.T) {
	rootDir := t.TempDir()
	serviceDir := filepath.Join(rootDir, "service")
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	startedPath := filepath.Join(rootDir, "started.txt")
	stoppedPath := filepath.Join(rootDir, "stopped.txt")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	proxyPort := listener.Addr().(*net.TCPAddr).Port
	configBody := strings.Join([]string{
		"repos:",
		"  capture: service",
		"  platform: ''",
		"  worker: ''",
		"ports:",
		"  capture: 0",
		"  worker: 0",
		"  proxy: " + strconv.Itoa(proxyPort),
		"sandbox:",
		"  state_dir: .state",
		"  project_id: ''",
		"  token: ''",
		"stack:",
		"  start_commands:",
		"    - repo: capture",
		"      args: [sh, -c, 'touch " + startedPath + "']",
		"  stop_commands:",
		"    - repo: capture",
		"      args: [sh, -c, 'touch " + stoppedPath + "']",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(rootDir, ".honch-sandbox.yaml"), []byte(configBody), 0o600); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "start", "--skip-migrations"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err = root.Execute()
	if err == nil {
		t.Fatal("start succeeded even though proxy port was already occupied")
	}
	if _, err := os.Stat(startedPath); err != nil {
		t.Fatalf("start command did not run before proxy failure: %v", err)
	}
	if _, err := os.Stat(stoppedPath); err != nil {
		t.Fatalf("start failure did not roll back the stack: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, ".state", "session.json")); !os.IsNotExist(err) {
		t.Fatalf("failed start left session state behind, stat err: %v", err)
	}
}

func TestSandboxStartRollsBackStackWhenServiceStartFails(t *testing.T) {
	rootDir := t.TempDir()
	serviceDir := filepath.Join(rootDir, "service")
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	startedPath := filepath.Join(rootDir, "started.txt")
	stoppedPath := filepath.Join(rootDir, "stopped.txt")
	configBody := strings.Join([]string{
		"repos:",
		"  capture: service",
		"  platform: ''",
		"  worker: ''",
		"sandbox:",
		"  state_dir: .state",
		"  project_id: ''",
		"  token: ''",
		"stack:",
		"  start_commands:",
		"    - repo: capture",
		"      args: [sh, -c, 'touch " + startedPath + " && exit 17']",
		"  stop_commands:",
		"    - repo: capture",
		"      args: [sh, -c, 'touch " + stoppedPath + "']",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(rootDir, ".honch-sandbox.yaml"), []byte(configBody), 0o600); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "start", "--skip-migrations"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("start succeeded even though the stack start command failed")
	}
	if _, err := os.Stat(startedPath); err != nil {
		t.Fatalf("start command did not run before failure: %v", err)
	}
	if _, err := os.Stat(stoppedPath); err != nil {
		t.Fatalf("service start failure did not roll back the stack: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, ".state", "session.json")); !os.IsNotExist(err) {
		t.Fatalf("failed start left session state behind, stat err: %v", err)
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
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	proxyPort := listener.Addr().(*net.TCPAddr).Port
	configBody := "ports:\n  proxy: " + strconv.Itoa(proxyPort) + "\n"
	if err := os.WriteFile(filepath.Join(rootDir, ".honch-sandbox.yaml"), []byte(configBody), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := session.NewManager(filepath.Join(rootDir, ".honch-sandbox", "session.json"))
	if err := manager.Save(session.State{
		Stack:  session.StackState{Running: true},
		Runner: session.RunnerState{Adapter: "c-core", PID: os.Getpid(), Detached: true},
		Proxy:  session.ProxyState{Mode: "online", Port: proxyPort},
	}); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "run", "c-core", "--detach"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	execErr := root.Execute()
	if execErr == nil {
		t.Fatal("run accepted a second active runner")
	}
	combined := execErr.Error() + "\n" + out.String()
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

func TestRunHelpUsesGenericAdapterPlaceholder(t *testing.T) {
	root := NewRootCommand(Dependencies{})
	root.SetArgs([]string{"--plain", "sandbox", "run", "--help"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("help returned error: %v", err)
	}
	help := ui.StripANSI(out.String())
	if !strings.Contains(help, "honch sandbox run <adapter> [--detach]") {
		t.Fatalf("run help did not use generic adapter placeholder:\n%s", help)
	}
	if strings.Contains(help, "run <c-core|esp-idf>") {
		t.Fatalf("run help hardcoded concrete adapter names:\n%s", help)
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
	for _, want := range []string{"no active sandbox session", "honch sandbox start", "honch sandbox run <adapter> --detach", "honch sandbox adapters list"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("control error missing %q:\n%s", want, combined)
		}
	}
	if strings.Contains(combined, "honch sandbox run c-core --detach") {
		t.Fatalf("control error used adapter-specific guidance:\n%s", combined)
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
	for _, want := range []string{"no active sandbox runner", "honch sandbox run <adapter> --detach", "honch sandbox adapters list", "honch sandbox status"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("control error missing %q:\n%s", want, combined)
		}
	}
	if strings.Contains(combined, "honch sandbox run c-core --detach") {
		t.Fatalf("control error used adapter-specific guidance:\n%s", combined)
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

func TestRunCommandRejectsUnknownAdapterWithRegistryNames(t *testing.T) {
	rootDir := t.TempDir()
	writeAdapterRegistryForTest(t, rootDir)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	proxyPort := listener.Addr().(*net.TCPAddr).Port
	if err := os.WriteFile(filepath.Join(rootDir, ".honch-sandbox.yaml"), []byte("ports:\n  proxy: "+strconv.Itoa(proxyPort)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := session.NewManager(filepath.Join(rootDir, ".honch-sandbox", "session.json"))
	if err := manager.Save(session.State{
		Stack: session.StackState{Running: true},
		Proxy: session.ProxyState{Mode: "online", Port: proxyPort},
	}); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "run", "micropython"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	execErr := root.Execute()
	if execErr == nil {
		t.Fatal("run accepted unknown adapter")
	}
	for _, want := range []string{"unsupported adapter", "c-core", "esp-idf"} {
		if !strings.Contains(execErr.Error(), want) {
			t.Fatalf("run error missing %q:\n%s", want, execErr.Error())
		}
	}
}

func TestRunnerServeResolvesAdapterKindFromRegistry(t *testing.T) {
	rootDir := t.TempDir()
	adaptersDir := filepath.Join(rootDir, "adapters")
	if err := os.MkdirAll(adaptersDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(adaptersDir, "host-smoke.yaml"), []byte("name: host-smoke\nkind: posix\nharness: harnesses/c-core\nbuild:\n  tool: cmake\ncontrols:\n  transport: newline-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	adapterConfig, serve, err := runnerSupervisorForAdapter(rootDir, "host-smoke")
	if err != nil {
		t.Fatalf("runnerSupervisorForAdapter returned error: %v", err)
	}
	if adapterConfig.Kind != "posix" {
		t.Fatalf("adapter kind = %q, want posix", adapterConfig.Kind)
	}
	if serve == nil {
		t.Fatal("serve function was nil")
	}
}

func TestSandboxRunnerProcessPatternsUseGenericSupervisorPattern(t *testing.T) {
	patterns := sandboxRunnerProcessPatterns("/repo", config.Config{Sandbox: config.SandboxConfig{StateDir: ".state"}})
	joined := strings.Join(patterns, "\n")
	if !strings.Contains(joined, "sandbox runner-serve ") {
		t.Fatalf("patterns did not include generic runner supervisor cleanup:\n%s", joined)
	}
	for _, adapterName := range []string{"runner-serve c-core", "runner-serve esp-idf"} {
		if strings.Contains(joined, adapterName) {
			t.Fatalf("patterns hardcoded adapter supervisor %q:\n%s", adapterName, joined)
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
	if strings.Contains(help, "install") {
		t.Fatalf("help advertised hidden install command:\n%s", help)
	}
	for _, want := range []string{
		"  honch",
		"    onboarding",
		"    Tools",
		"      sandbox    ›   Run the Honch SDK E2E sandbox",
	} {
		if !strings.Contains(ui.StripANSI(help), want) {
			t.Fatalf("help missing %q:\n%s", want, ui.StripANSI(help))
		}
	}
}

func TestHiddenInstallCommandCopiesBinaryToLocalBin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	root := NewRootCommand(Dependencies{In: bytes.NewBufferString("y\n")})
	root.SetArgs([]string{"--plain", "install"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("install returned error: %v\n%s", err, out.String())
	}

	target := filepath.Join(home, ".local", "bin", "honch")
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("installed binary missing: %v\n%s", err, out.String())
	}
	if info.IsDir() {
		t.Fatalf("installed path is a directory: %s", target)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("installed binary is not executable: %#o", info.Mode())
	}
	combined := out.String()
	for _, want := range []string{
		"Install honch to " + target + "? [y/N]",
		"Installed honch to " + target,
		"Reload your shell or run `hash -r`",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("install output missing %q:\n%s", want, combined)
		}
	}
}

func TestHiddenInstallCommandCanBeCancelled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	root := NewRootCommand(Dependencies{In: bytes.NewBufferString("n\n")})
	root.SetArgs([]string{"--plain", "install"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("install succeeded even though confirmation was declined")
	}
	if !strings.Contains(err.Error(), "install cancelled") {
		t.Fatalf("install error did not explain cancellation: %v", err)
	}

	target := filepath.Join(home, ".local", "bin", "honch")
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("install created a binary after cancellation, stat err: %v", statErr)
	}
}

func TestOnboardingAutoLaunchRunsOnce(t *testing.T) {
	rootDir := t.TempDir()
	for _, path := range []string{
		filepath.Join(rootDir, "go.mod"),
		filepath.Join(rootDir, "adapters"),
		filepath.Join(rootDir, "harnesses"),
	} {
		if strings.HasSuffix(path, "go.mod") {
			if err := os.WriteFile(path, []byte("module example.com/test\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	prevGate := onboardingGate
	onboardingGate = func(io.Reader, io.Writer) bool { return true }
	t.Cleanup(func() { onboardingGate = prevGate })

	root := NewRootCommand(Dependencies{RootDir: rootDir, In: bytes.NewBufferString("\nn\nn\nn\nn\n")})
	root.SetArgs([]string{"--plain"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("first launch onboarding returned error: %v\n%s", err, out.String())
	}
	marker := filepath.Join(rootDir, ".honch-sandbox", "onboarding.json")
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("onboarding marker missing: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Honch onboarding") {
		t.Fatalf("first launch did not show onboarding:\n%s", out.String())
	}

	second := NewRootCommand(Dependencies{RootDir: rootDir})
	second.SetArgs([]string{"--plain"})
	var secondOut bytes.Buffer
	second.SetOut(&secondOut)
	second.SetErr(&secondOut)

	if err := second.Execute(); err != nil {
		t.Fatalf("second launch returned error: %v\n%s", err, secondOut.String())
	}
	if strings.Contains(secondOut.String(), "Honch onboarding") {
		t.Fatalf("onboarding repeated after completion:\n%s", secondOut.String())
	}
}

func TestOnboardingCommandUsesGuidedStepFlow(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootDir, "go.mod"), []byte("module example.com/test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(rootDir, "adapters"),
		filepath.Join(rootDir, "harnesses"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	root := NewRootCommand(Dependencies{RootDir: rootDir, In: bytes.NewBufferString("\nn\nn\nn\nn\n")})
	root.SetArgs([]string{"--plain", "onboarding"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("onboarding returned error: %v\n%s", err, out.String())
	}
	output := ui.StripANSI(out.String())
	if strings.Contains(output, "Honch setup status") {
		t.Fatalf("onboarding still dumps setup status before the guided flow:\n%s", output)
	}
	assertContainsInOrder(t, output, []string{
		"Step 1 of 4: Welcome",
		"Continue onboarding? [Enter/q]",
		"Step 2 of 4: Repositories",
		"Clone missing Honch repos now? [y/N/b/q]",
		"Update sibling repo paths now? [y/N/b/q]",
		"Step 3 of 4: Setup",
		"Run the recommended sandbox setup now? [y/N/b/q]",
		"Step 4 of 4: Install",
		"Install honch to",
		"Honch onboarding complete",
	})
}

func TestOnboardingCommandCanExitBeforeSavingMarker(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootDir, "go.mod"), []byte("module example.com/test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(rootDir, "adapters"),
		filepath.Join(rootDir, "harnesses"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	root := NewRootCommand(Dependencies{RootDir: rootDir, In: bytes.NewBufferString("q\n")})
	root.SetArgs([]string{"--plain", "onboarding"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("onboarding returned error: %v\n%s", err, out.String())
	}
	output := ui.StripANSI(out.String())
	if !strings.Contains(output, "Onboarding exited") {
		t.Fatalf("onboarding did not explain exit:\n%s", output)
	}
	marker := filepath.Join(rootDir, ".honch-sandbox", "onboarding.json")
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("onboarding marker should not exist after exit, stat err: %v", err)
	}
}

func TestAutoOnboardingExitStopsRequestedCommand(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootDir, "go.mod"), []byte("module example.com/test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(rootDir, "adapters"),
		filepath.Join(rootDir, "harnesses"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	prevGate := onboardingGate
	onboardingGate = func(io.Reader, io.Writer) bool { return true }
	t.Cleanup(func() { onboardingGate = prevGate })

	root := NewRootCommand(Dependencies{RootDir: rootDir, In: bytes.NewBufferString("q\n")})
	root.SetArgs([]string{"--plain", "sandbox", "status"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("command succeeded after onboarding exit")
	}
	output := ui.StripANSI(out.String())
	if !strings.Contains(output, "Onboarding exited") {
		t.Fatalf("onboarding did not explain exit:\n%s", output)
	}
	if strings.Contains(output, "Honch sandbox status") {
		t.Fatalf("requested command ran after onboarding exit:\n%s", output)
	}
}

func TestOnboardingCommandCanGoBackToPreviousStep(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootDir, "go.mod"), []byte("module example.com/test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(rootDir, "adapters"),
		filepath.Join(rootDir, "harnesses"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	root := NewRootCommand(Dependencies{RootDir: rootDir, In: bytes.NewBufferString("\nn\nn\nb\nn\nn\nn\n")})
	root.SetArgs([]string{"--plain", "onboarding"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("onboarding returned error: %v\n%s", err, out.String())
	}
	output := ui.StripANSI(out.String())
	firstRepos := strings.Index(output, "Step 2 of 4: Repositories")
	setup := strings.Index(output, "Step 3 of 4: Setup")
	lastRepos := strings.LastIndex(output, "Step 2 of 4: Repositories")
	if firstRepos < 0 || setup < 0 || lastRepos <= setup {
		t.Fatalf("onboarding did not return from setup to repositories:\n%s", output)
	}
}

func TestOnboardingCommandSavesRepoPaths(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootDir, "go.mod"), []byte("module example.com/test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(rootDir, "adapters"),
		filepath.Join(rootDir, "harnesses"),
		filepath.Join(rootDir, "repos", "capture"),
		filepath.Join(rootDir, "repos", "platform"),
		filepath.Join(rootDir, "repos", "worker"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	capture := filepath.Join(rootDir, "repos", "capture")
	platform := filepath.Join(rootDir, "repos", "platform")
	worker := filepath.Join(rootDir, "repos", "worker")

	root := NewRootCommand(Dependencies{RootDir: rootDir, In: bytes.NewBufferString("\nn\ny\n" + capture + "\n" + platform + "\n" + worker + "\nn\nn\n")})
	root.SetArgs([]string{"--plain", "onboarding"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("onboarding returned error: %v\n%s", err, out.String())
	}
	data, err := os.ReadFile(filepath.Join(rootDir, ".honch-sandbox.yaml"))
	if err != nil {
		t.Fatalf("read updated config: %v\n%s", err, out.String())
	}
	for _, want := range []string{capture, platform, worker} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("updated config missing %q:\n%s", want, string(data))
		}
	}
}

func TestOnboardingCommandCanCloneMissingRepos(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootDir, "go.mod"), []byte("module example.com/test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(rootDir, "adapters"),
		filepath.Join(rootDir, "harnesses"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	var calls []string
	prevClone := cloneSiblingRepo
	cloneSiblingRepo = func(ctx context.Context, stdout io.Writer, stderr io.Writer, source siblingRepoSource, target string) error {
		calls = append(calls, source.Name+"="+source.URL+"->"+target)
		return os.MkdirAll(target, 0o755)
	}
	t.Cleanup(func() { cloneSiblingRepo = prevClone })

	root := NewRootCommand(Dependencies{RootDir: rootDir, In: bytes.NewBufferString("\ny\n\nn\nn\n")})
	root.SetArgs([]string{"--plain", "onboarding"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("onboarding returned error: %v\n%s", err, out.String())
	}
	parent := filepath.Dir(rootDir)
	wants := []string{
		"capture=https://github.com/honch-io/capture.git->" + filepath.Join(parent, "capture"),
		"platform=https://github.com/honch-io/platform.git->" + filepath.Join(parent, "platform"),
		"worker=https://github.com/honch-io/worker.git->" + filepath.Join(parent, "worker"),
	}
	if strings.Join(calls, "\n") != strings.Join(wants, "\n") {
		t.Fatalf("clone calls mismatch:\n got: %q\nwant: %q", calls, wants)
	}
	data, err := os.ReadFile(filepath.Join(rootDir, ".honch-sandbox.yaml"))
	if err != nil {
		t.Fatalf("read updated config: %v\n%s", err, out.String())
	}
	for _, want := range []string{
		"capture: " + filepath.Join(parent, "capture"),
		"platform: " + filepath.Join(parent, "platform"),
		"worker: " + filepath.Join(parent, "worker"),
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("updated config missing %q:\n%s", want, string(data))
		}
	}
	for _, want := range []string{"Clone missing Honch repos now? [y/N/b/q]", "Clone destination parent"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("onboarding output missing %q:\n%s", want, out.String())
		}
	}
}

func TestInstallScriptBootstrapsLatestReleaseAndRunsOnboarding(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "scripts", "install.sh"))
	if err != nil {
		t.Fatalf("read install script: %v", err)
	}
	script := string(data)
	for _, want := range []string{
		"releases/latest/download/honch-${os_name}-${arch_name}",
		"curl -fL",
		"~/.local/bin",
		"~/.local/share/honch/sandbox",
		"git clone",
		"cd \"$sandbox_dir\"",
		"honch onboarding",
		"--no-install",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("install script missing %q:\n%s", want, script)
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
	if _, err := parseProperties(`null`); err == nil {
		t.Fatal("parseProperties accepted JSON null")
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

	proc, err := startProxyProcess(context.Background(), rootDir, cfg, strings.NewReader(""), io.Discard, io.Discard)
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

func TestStartProxyProcessRejectsPortWithStaleLivePID(t *testing.T) {
	rootDir := t.TempDir()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	cfg := configForTest()
	cfg.Ports.Proxy = listener.Addr().(*net.TCPAddr).Port
	if err := writePIDFile(proxyPIDPath(rootDir, cfg), os.Getpid()); err != nil {
		t.Fatal(err)
	}

	proc, err := startProxyProcess(context.Background(), rootDir, cfg, strings.NewReader(""), io.Discard, io.Discard)
	if err == nil {
		t.Fatal("startProxyProcess accepted an unrelated live PID as proxy owner")
	}
	if proc != nil {
		t.Fatalf("startProxyProcess returned process for unrelated listener: %+v", proc)
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
	dir := filepath.Join(root, "adapters")
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
