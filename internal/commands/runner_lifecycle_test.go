package commands

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/honch/sdk/tools/sandbox/internal/session"
)

func TestSandboxStopStopsFullStackAndClearsSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group cleanup is POSIX-only")
	}
	rootDir := t.TempDir()
	writeAdapterRegistryForTest(t, rootDir)
	parentDir := filepath.Dir(rootDir)
	platformInfra := filepath.Join(parentDir, "platform", "infra")
	if err := os.MkdirAll(platformInfra, 0o755); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(rootDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dockerPath := filepath.Join(binDir, "docker")
	if err := os.WriteFile(dockerPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	cCorePath := filepath.Join(rootDir, ".honch-sandbox", "build", "c-core", "honch_sandbox_c_core")
	writeSleepScript(t, cCorePath)
	cCoreCmd := startScriptProcess(t, cCorePath)
	stubSandboxProcessIDs(t, cCoreCmd.Process.Pid, cCorePath)
	killCalls := stubKillProcessCalls(t)
	controlPath := filepath.Join(rootDir, ".honch-sandbox", "c-core.control")
	if err := os.WriteFile(controlPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	proxyPath := filepath.Join(rootDir, "bin", "proxy-sleep")
	writeSleepScript(t, proxyPath)
	proxyCmd := startScriptProcess(t, proxyPath)
	manager := session.NewManager(filepath.Join(rootDir, ".honch-sandbox", "session.json"))
	if err := manager.Save(session.State{
		Stack:  session.StackState{Running: true},
		Runner: session.RunnerState{Adapter: "c-core", PID: cCoreCmd.Process.Pid, Detached: true, ControlPath: controlPath},
		Proxy:  session.ProxyState{Mode: "online", PID: proxyCmd.Process.Pid},
	}); err != nil {
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
		t.Fatalf("stop did not report completion:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(rootDir, ".honch-sandbox", "session.json")); !os.IsNotExist(err) {
		t.Fatalf("stop did not clear session, stat err: %v", err)
	}
	if _, err := os.Stat(controlPath); !os.IsNotExist(err) {
		t.Fatalf("stop did not remove control file, stat err: %v", err)
	}
	for _, want := range []int{cCoreCmd.Process.Pid, proxyCmd.Process.Pid} {
		if !containsInt(*killCalls, want) {
			t.Fatalf("stop did not target pid %d: %v", want, *killCalls)
		}
	}
}

func TestSandboxStopAdapterPreservesStackState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group cleanup is POSIX-only")
	}
	rootDir := t.TempDir()
	writeAdapterRegistryForTest(t, rootDir)
	cCorePath := filepath.Join(rootDir, ".honch-sandbox", "build", "c-core", "honch_sandbox_c_core")
	writeSleepScript(t, cCorePath)
	cCoreCmd := startScriptProcess(t, cCorePath)
	stubSandboxProcessIDs(t, cCoreCmd.Process.Pid, cCorePath)
	killCalls := stubKillProcessCalls(t)
	controlPath := filepath.Join(rootDir, ".honch-sandbox", "c-core.control")
	if err := os.WriteFile(controlPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	manager := session.NewManager(filepath.Join(rootDir, ".honch-sandbox", "session.json"))
	if err := manager.Save(session.State{
		Stack:  session.StackState{Running: true},
		Runner: session.RunnerState{Adapter: "c-core", PID: cCoreCmd.Process.Pid, Detached: true, ControlPath: controlPath},
		Proxy:  session.ProxyState{Mode: "online"},
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "stop", "c-core"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("stop c-core returned error: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(controlPath); !os.IsNotExist(err) {
		t.Fatalf("stop c-core did not remove control file, stat err: %v", err)
	}
	state, err := manager.Load()
	if err != nil {
		t.Fatalf("stop c-core removed the session: %v", err)
	}
	if !state.Stack.Running {
		t.Fatal("stop c-core cleared the stack state")
	}
	if state.Runner.Adapter != "" || state.Runner.PID != 0 || state.Runner.ControlPath != "" {
		t.Fatalf("stop c-core left runner state behind: %+v", state.Runner)
	}
	if !containsInt(*killCalls, cCoreCmd.Process.Pid) {
		t.Fatalf("stop c-core did not target pid %d: %v", cCoreCmd.Process.Pid, *killCalls)
	}
}

func TestQEMUStopAliasStopsEspIDFRunner(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group cleanup is POSIX-only")
	}
	rootDir := t.TempDir()
	writeAdapterRegistryForTest(t, rootDir)
	flashPath := filepath.Join(rootDir, ".honch-sandbox", "build", "esp-idf", "qemu_flash.bin")
	if err := os.MkdirAll(filepath.Dir(flashPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(flashPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	qemuPath := filepath.Join(rootDir, "bin", "qemu-system-xtensa")
	writeQEMUScript(t, qemuPath)
	qemuCmd := startScriptProcess(t, qemuPath, "-M", "esp32", "-drive", "file="+flashPath+",if=mtd,format=raw")
	stubSandboxProcessIDs(t, qemuCmd.Process.Pid, flashPath)
	killCalls := stubKillProcessCalls(t)
	controlPath := filepath.Join(rootDir, ".honch-sandbox", "esp-idf.control")
	if err := os.WriteFile(controlPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	manager := session.NewManager(filepath.Join(rootDir, ".honch-sandbox", "session.json"))
	if err := manager.Save(session.State{
		Stack:  session.StackState{Running: true},
		Runner: session.RunnerState{Adapter: "esp-idf", PID: qemuCmd.Process.Pid, Detached: true, ControlPath: controlPath},
		Proxy:  session.ProxyState{Mode: "online"},
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "qemu", "stop"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("qemu stop returned error: %v\n%s", err, out.String())
	}
	state, err := manager.Load()
	if err != nil {
		t.Fatalf("qemu stop removed the session: %v", err)
	}
	if _, err := os.Stat(controlPath); !os.IsNotExist(err) {
		t.Fatalf("qemu stop did not remove control file, stat err: %v", err)
	}
	if state.Runner.Adapter != "" || state.Runner.PID != 0 || state.Runner.ControlPath != "" {
		t.Fatalf("qemu stop left runner state behind: %+v", state.Runner)
	}
	if !containsInt(*killCalls, qemuCmd.Process.Pid) {
		t.Fatalf("qemu stop did not target pid %d: %v", qemuCmd.Process.Pid, *killCalls)
	}
}

func TestSandboxStatusReportsQEMUUp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group cleanup is POSIX-only")
	}
	rootDir := t.TempDir()
	writeAdapterRegistryForTest(t, rootDir)
	flashPath := filepath.Join(rootDir, ".honch-sandbox", "build", "esp-idf", "qemu_flash.bin")
	if err := os.MkdirAll(filepath.Dir(flashPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(flashPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	qemuPath := filepath.Join(rootDir, "bin", "qemu-system-xtensa")
	writeQEMUScript(t, qemuPath)
	qemuCmd := startScriptProcess(t, qemuPath, "-M", "esp32", "-drive", "file="+flashPath+",if=mtd,format=raw")
	stubSandboxProcessIDs(t, qemuCmd.Process.Pid, flashPath)
	t.Cleanup(func() {
		_ = killProcess(qemuCmd.Process.Pid)
		_ = qemuCmd.Wait()
	})
	manager := session.NewManager(filepath.Join(rootDir, ".honch-sandbox", "session.json"))
	if err := manager.Save(session.State{
		Stack:  session.StackState{Running: true},
		Runner: session.RunnerState{Adapter: "esp-idf", PID: qemuCmd.Process.Pid, Detached: true},
		Proxy:  session.ProxyState{Mode: "online"},
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand(Dependencies{RootDir: rootDir})
	root.SetArgs([]string{"--plain", "sandbox", "status"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("status returned error: %v\n%s", err, out.String())
	}
	combined := out.String()
	for _, want := range []string{"qemu", "up"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("status did not report qemu as up:\n%s", combined)
		}
	}
}

func TestQEMUHelpShowsStopCommand(t *testing.T) {
	root := NewRootCommand(Dependencies{})
	root.SetArgs([]string{"sandbox", "qemu", "--help"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("help returned error: %v", err)
	}
	help := out.String()
	for _, want := range []string{"doctor", "install", "stop"} {
		if !strings.Contains(help, want) {
			t.Fatalf("qemu help missing %q:\n%s", want, help)
		}
	}
}

func TestSandboxStopHelpShowsAdapterUsage(t *testing.T) {
	root := NewRootCommand(Dependencies{})
	root.SetArgs([]string{"sandbox", "stop", "--help"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("help returned error: %v", err)
	}
	help := out.String()
	for _, want := range []string{"honch sandbox stop [adapter]"} {
		if !strings.Contains(help, want) {
			t.Fatalf("stop help missing %q:\n%s", want, help)
		}
	}
}

func writeSleepScript(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "#!/bin/sh\ntrap 'exit 0' INT TERM\nsleep 30\n"
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
}

func writeQEMUScript(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "#!/bin/sh\ntrap 'exit 0' INT TERM\nsleep 30\n"
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
}

func startScriptProcess(t *testing.T, path string, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = killProcess(cmd.Process.Pid)
			_ = cmd.Wait()
		}
	})
	return cmd
}

func stubSandboxProcessIDs(t *testing.T, pid int, needles ...string) {
	t.Helper()
	prev := sandboxProcessIDsFn
	sandboxProcessIDsFn = func(pattern string) []int {
		for _, needle := range needles {
			if strings.Contains(pattern, needle) {
				return []int{pid}
			}
		}
		return nil
	}
	t.Cleanup(func() {
		sandboxProcessIDsFn = prev
	})
}

func stubKillProcessCalls(t *testing.T) *[]int {
	t.Helper()
	prev := killProcessFn
	calls := []int{}
	killProcessFn = func(pid int) error {
		calls = append(calls, pid)
		return nil
	}
	t.Cleanup(func() {
		killProcessFn = prev
	})
	return &calls
}

func containsInt(values []int, want int) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
