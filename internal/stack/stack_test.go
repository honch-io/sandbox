package stack

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"honch.dev/honch/internal/config"
)

func TestBackgroundPortServerHelper(t *testing.T) {
	if os.Getenv("HONCH_STACK_PORT_HELPER") != "1" {
		return
	}
	if len(os.Args) == 0 {
		os.Exit(2)
	}
	port, err := strconv.Atoi(os.Args[len(os.Args)-1])
	if err != nil {
		os.Exit(2)
	}
	signal.Ignore(os.Interrupt)
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		os.Exit(2)
	}
	defer func() {
		_ = listener.Close()
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		_ = conn.Close()
	}
}

func TestStartRunsBackgroundCommandsFromConfiguredSubdirectory(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "service")
	workdir := filepath.Join(repo, "infra")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "ran.txt")

	cfg := config.Config{
		Repos:   config.ReposConfig{Capture: "service"},
		Sandbox: config.SandboxConfig{StateDir: ".state"},
		Stack: config.StackConfig{StartCommands: []config.CommandConfig{
			{
				Repo:       "capture",
				WorkingDir: "infra",
				Args:       []string{"sh", "-c", "pwd > " + output},
				Background: true,
				Log:        "capture.log",
			},
		}},
	}
	service := New(root)

	if err := service.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	var data []byte
	var err error
	for i := 0; i < 20; i++ {
		data, err = os.ReadFile(output)
		if err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("background command did not write output: %v", err)
	}
	want, err := filepath.EvalSymlinks(workdir)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != want+"\n" {
		t.Fatalf("background command ran in %q, want %q", got, want+"\n")
	}
	if _, err := os.Stat(filepath.Join(root, ".state", "pids", "capture.pid")); err != nil {
		t.Fatalf("background pid file missing: %v", err)
	}
}

func TestStartBackgroundCommandPassesConfiguredEnv(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "platform")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "env.txt")

	cfg := config.Config{
		Repos:   config.ReposConfig{Platform: "platform"},
		Sandbox: config.SandboxConfig{StateDir: ".state"},
		Stack: config.StackConfig{StartCommands: []config.CommandConfig{
			{
				Repo: "platform",
				Args: []string{"sh", "-c", "printf '%s\\n' \"$PUBSUB_EMULATOR_HOST\" \"$REDIS_URL\" > " + output},
				Env: map[string]string{
					"PUBSUB_EMULATOR_HOST": "localhost:8085",
					"REDIS_URL":            "redis://localhost:6379",
				},
				Background: true,
				Log:        "platform.log",
			},
		}},
	}

	service := New(root)
	service.SkipMigrations = true
	if err := service.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	var data []byte
	var err error
	for i := 0; i < 20; i++ {
		data, err = os.ReadFile(output)
		if err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("background command did not write env: %v", err)
	}
	if got, want := string(data), "localhost:8085\nredis://localhost:6379\n"; got != want {
		t.Fatalf("env output = %q, want %q", got, want)
	}
}

func TestStartDockerCommandUsesConfiguredDockerHost(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "platform", "infra")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "docker-host.txt")
	docker := filepath.Join(binDir, "docker")
	script := "#!/bin/sh\nprintf '%s\\n' \"$DOCKER_HOST\" > " + output + "\n"
	if err := os.WriteFile(docker, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	cfg := config.Config{
		Repos:   config.ReposConfig{Platform: "platform"},
		Sandbox: config.SandboxConfig{StateDir: ".state", DockerHost: "ssh://docker.example"},
		Stack: config.StackConfig{StartCommands: []config.CommandConfig{
			{
				Repo:       "platform",
				WorkingDir: "infra",
				Args:       []string{"docker", "compose", "up", "-d"},
			},
		}},
	}
	service := New(root)
	service.SkipMigrations = true

	if err := service.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "ssh://docker.example\n"; got != want {
		t.Fatalf("DOCKER_HOST = %q, want %q", got, want)
	}
}

func TestStartWaitsForPostgresReadinessBeforeMigrationsAndSeed(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "platform")
	if err := os.MkdirAll(filepath.Join(repo, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "docker.log")
	readyPath := filepath.Join(root, "postgres.ready")
	docker := filepath.Join(binDir, "docker")
	dockerScript := "#!/bin/sh\n" +
		"echo \"$*\" >> " + logPath + "\n" +
		"if [ \"$1\" = exec ] && [ \"$3\" = infra-postgres-1 ] && [ \"$4\" = pg_isready ]; then\n" +
		"  if [ ! -f " + readyPath + " ]; then exit 1; fi\n" +
		"  exit 0\n" +
		"fi\n" +
		"if [ \"$1\" = exec ] && [ \"$3\" = infra-postgres-1 ] && [ \"$4\" = psql ]; then\n" +
		"  if [ ! -f " + readyPath + " ]; then\n" +
		"    echo psql-before-ready >&2\n" +
		"    exit 1\n" +
		"  fi\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(docker, []byte(dockerScript), 0o700); err != nil {
		t.Fatal(err)
	}
	bun := filepath.Join(binDir, "bun")
	if err := os.WriteFile(bun, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	go func() {
		time.Sleep(500 * time.Millisecond)
		_ = os.WriteFile(readyPath, []byte("ready"), 0o600)
	}()

	cfg := config.Config{
		Repos:   config.ReposConfig{Platform: "platform"},
		Sandbox: config.SandboxConfig{StateDir: ".state", ProjectID: "11111111-1111-1111-1111-111111111111", Token: "sandbox-token"},
		Stack: config.StackConfig{
			StartCommands: nil,
		},
	}
	service := New(root)
	service.ApproveMigrations = func() (bool, error) {
		return true, nil
	}

	if err := service.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(logData)
	firstReady := strings.Index(log, "pg_isready")
	firstSeed := strings.Index(log, "psql -U platform -d platform -c")
	if firstReady == -1 || firstSeed == -1 {
		t.Fatalf("missing readiness or seed commands:\n%s", log)
	}
	if firstSeed < firstReady {
		t.Fatalf("database seed ran before readiness probe:\n%s", log)
	}
}

func TestStartBackgroundCommandReleasesProcessAfterPIDWrite(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 0")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var wrotePID int

	if err := startBackgroundCommand(cmd, func(pid int) error {
		wrotePID = pid
		return nil
	}); err != nil {
		t.Fatalf("startBackgroundCommand returned error: %v", err)
	}
	if wrotePID <= 0 {
		t.Fatalf("pid was not written: %d", wrotePID)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("released background process remained waitable")
	}
}

func TestStopRemovesBackgroundPidFiles(t *testing.T) {
	root := t.TempDir()
	pidDir := filepath.Join(root, ".state", "pids")
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pidDir, "capture.pid"), []byte("999999"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Sandbox: config.SandboxConfig{StateDir: ".state"}}

	if err := New(root).Stop(context.Background(), cfg); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(pidDir, "capture.pid")); !os.IsNotExist(err) {
		t.Fatalf("pid file still exists or unexpected error: %v", err)
	}
}

func TestStopRemovesMalformedBackgroundPidFiles(t *testing.T) {
	root := t.TempDir()
	pidDir := filepath.Join(root, ".state", "pids")
	pidPath := filepath.Join(pidDir, "capture.pid")
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidPath, []byte("not-a-pid"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Sandbox: config.SandboxConfig{StateDir: ".state"}}

	if err := New(root).Stop(context.Background(), cfg); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("malformed pid file still exists or unexpected error: %v", err)
	}
}

func TestStopRunsStopCommandsWhenBackgroundShutdownFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group cleanup is POSIX-only")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "platform")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = listener.Close()
	}()
	cmd := exec.Command("sh", "-c", "sleep 30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			_ = cmd.Wait()
		}
	})
	pidDir := filepath.Join(root, ".state", "pids")
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pidDir, "capture.pid"), []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "stop-ran")
	cfg := config.Config{
		Repos:   config.ReposConfig{Platform: "platform"},
		Ports:   config.PortsConfig{Capture: listener.Addr().(*net.TCPAddr).Port},
		Sandbox: config.SandboxConfig{StateDir: ".state"},
		Stack: config.StackConfig{StopCommands: []config.CommandConfig{
			{Repo: "platform", Args: []string{"sh", "-c", "touch " + marker}},
		}},
	}

	if err := New(root).Stop(context.Background(), cfg); err == nil {
		t.Fatal("Stop succeeded despite background shutdown failure")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("stop command did not run after background shutdown failure: %v", err)
	}
}

func TestStartSkipsMigrationsWhenMigrationDeclined(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "platform")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "ran.txt")
	cfg := config.Config{
		Repos:   config.ReposConfig{Platform: "platform"},
		Sandbox: config.SandboxConfig{StateDir: ".state"},
		Stack: config.StackConfig{StartCommands: []config.CommandConfig{
			{
				Repo: "platform",
				Args: []string{"sh", "-c", "touch " + output},
			},
		}},
	}
	service := New(root)
	service.ApproveMigrations = func() (bool, error) {
		return false, nil
	}

	err := service.Start(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("start command did not run after migration decline: %v", err)
	}
}

func TestStartRejectsUnownedServicePort(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "capture")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = listener.Close()
	}()
	port := listener.Addr().(*net.TCPAddr).Port
	cfg := config.Config{
		Repos:   config.ReposConfig{Capture: "capture"},
		Ports:   config.PortsConfig{Capture: port},
		Sandbox: config.SandboxConfig{StateDir: ".state"},
		Stack: config.StackConfig{StartCommands: []config.CommandConfig{
			{
				Repo:       "capture",
				Args:       []string{"sh", "-c", "sleep 1"},
				Background: true,
				Log:        "capture.log",
			},
		}},
	}

	err = New(root).Start(context.Background(), cfg)
	if err == nil {
		t.Fatal("Start accepted an occupied capture port without sandbox ownership")
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("error did not explain occupied port ownership: %v", err)
	}
}

func TestStartRejectsOccupiedServicePortWithStaleLivePID(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "capture")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = listener.Close()
	}()
	port := listener.Addr().(*net.TCPAddr).Port
	pidDir := filepath.Join(root, ".state", "pids")
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pidDir, "capture.pid"), []byte(fmt.Sprintf("%d", os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Repos:   config.ReposConfig{Capture: "capture"},
		Ports:   config.PortsConfig{Capture: port},
		Sandbox: config.SandboxConfig{StateDir: ".state"},
		Stack: config.StackConfig{StartCommands: []config.CommandConfig{
			{
				Repo:       "capture",
				Args:       []string{"sh", "-c", "sleep 1"},
				Background: true,
				Log:        "capture.log",
			},
		}},
	}

	err = New(root).Start(context.Background(), cfg)
	if err == nil {
		t.Fatal("Start accepted an occupied capture port with unrelated live PID")
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("error did not explain occupied port ownership: %v", err)
	}
}

func TestCommandMatchesArgsAcceptsCargoRunPackageBinary(t *testing.T) {
	if !commandMatchesArgs("target/debug/honch-capture", []string{"cargo", "run", "-p", "honch-capture"}) {
		t.Fatal("cargo package binary did not match cargo run package args")
	}
	if !commandMatchesArgs("cargo run -p honch-capture", []string{"cargo", "run", "-p", "honch-capture"}) {
		t.Fatal("literal cargo run command did not match args")
	}
	if commandMatchesArgs("target/debug/other-service", []string{"cargo", "run", "-p", "honch-capture"}) {
		t.Fatal("unrelated binary matched cargo package args")
	}
}

func TestStopReleasesTrackedBackgroundPorts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group cleanup is POSIX-only")
	}
	root := t.TempDir()
	port := freeTCPPort(t)
	cmd := exec.Command(os.Args[0], "-test.run=TestBackgroundPortServerHelper", "--", strconv.Itoa(port))
	cmd.Env = append(os.Environ(), "HONCH_STACK_PORT_HELPER=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			_ = cmd.Wait()
		}
	})
	waitUntilPortOpen(t, port)
	pidDir := filepath.Join(root, ".state", "pids")
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pidDir, "capture.pid"), []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Ports:   config.PortsConfig{Capture: port},
		Sandbox: config.SandboxConfig{StateDir: ".state"},
	}

	if err := New(root).Stop(context.Background(), cfg); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	if portOpen(context.Background(), port, 50*time.Millisecond) {
		t.Fatalf("Stop returned while capture port %d was still open", port)
	}
}

func TestWaitForBackgroundPortsAppliesTimeoutPerService(t *testing.T) {
	capturePort := freeTCPPort(t)
	workerPort := freeTCPPort(t)
	stopCapture := startDelayedTCPListener(t, capturePort, 500*time.Millisecond)
	defer stopCapture()
	stopWorker := startDelayedTCPListener(t, workerPort, 900*time.Millisecond)
	defer stopWorker()

	cfg := config.Config{
		Ports: config.PortsConfig{
			Capture: capturePort,
			Worker:  workerPort,
		},
	}

	if err := New(t.TempDir()).waitForBackgroundPorts(context.Background(), cfg, 700*time.Millisecond); err != nil {
		t.Fatalf("waitForBackgroundPorts returned error: %v", err)
	}
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = listener.Close()
	}()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitUntilPortOpen(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if portOpen(context.Background(), port, 50*time.Millisecond) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("port %d did not open", port)
}

func startDelayedTCPListener(t *testing.T, port int, delay time.Duration) func() {
	t.Helper()
	closed := make(chan struct{})
	ready := make(chan net.Listener, 1)
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-closed:
			return
		}
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			t.Errorf("listen on delayed port %d: %v", port, err)
			return
		}
		ready <- listener
		defer func() {
			_ = listener.Close()
		}()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	return func() {
		close(closed)
		select {
		case listener := <-ready:
			_ = listener.Close()
		default:
		}
	}
}
