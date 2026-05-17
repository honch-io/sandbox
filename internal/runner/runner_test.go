package runner

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func stubQEMUSerial(t *testing.T, writes []string, closeAfter time.Duration) {
	t.Helper()
	oldDial := dialQEMUSerialFn
	dialQEMUSerialFn = func(ctx context.Context, address string) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			for _, line := range writes {
				if _, err := io.WriteString(server, line); err != nil {
					return
				}
			}
			if closeAfter > 0 {
				time.Sleep(closeAfter)
			}
		}()
		return client, nil
	}
	t.Cleanup(func() {
		dialQEMUSerialFn = oldDial
	})
}

func TestStartReturnsBeforeProcessExits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test is POSIX-only")
	}
	script := filepath.Join(t.TempDir(), "sleep.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 2\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	cmd, err := Start(context.Background(), script, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("Start waited for the process to exit")
	}
}

func TestQEMUSerialArgUsesConfiguredHostAndPort(t *testing.T) {
	t.Setenv("HONCH_SANDBOX_QEMU_SERIAL_ADDR", "127.0.0.1:6200")

	if got := qemuSerialArg(); got != "tcp:127.0.0.1:6200,server,nowait" {
		t.Fatalf("qemuSerialArg = %q, want tcp:127.0.0.1:6200,server,nowait", got)
	}
}

func TestEspIDFRunnerBuildUsesIDFPyWithStateBuildDirAndSandboxDefines(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake idf.py script is POSIX-only")
	}
	repoRoot := t.TempDir()
	stateDir := filepath.Join(repoRoot, ".honch-sandbox")
	projectDir := filepath.Join(repoRoot, "harnesses", "esp-idf")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(repoRoot, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(repoRoot, "idf.log")
	idfPy := filepath.Join(binDir, "idf.py")
	script := "#!/bin/sh\nprintf '%s\\n' \"$PWD|$*\" >> " + logPath + "\n"
	if err := os.WriteFile(idfPy, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	r := EspIDFRunner{RepoRoot: repoRoot, StateDir: stateDir}
	build, err := r.Build(context.Background(), EspIDFSettings{
		Endpoint: "http://10.0.2.2:18080",
		Token:    "honch_e2e_test_key",
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	wantBuildDir := filepath.Join(stateDir, "build", "esp-idf")
	if build.ProjectDir != projectDir {
		t.Fatalf("ProjectDir = %q", build.ProjectDir)
	}
	if build.BuildDir != wantBuildDir {
		t.Fatalf("BuildDir = %q, want %q", build.BuildDir, wantBuildDir)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	wantSDKConfig := filepath.Join(wantBuildDir, "sdkconfig")
	for _, want := range []string{
		"-B " + wantBuildDir + " -D SDKCONFIG=" + wantSDKConfig + " -D HONCH_SANDBOX_HOST=http://10.0.2.2:18080 -D HONCH_SANDBOX_API_KEY=honch_e2e_test_key set-target esp32",
		"-B " + wantBuildDir + " -D SDKCONFIG=" + wantSDKConfig + " -D HONCH_SANDBOX_HOST=http://10.0.2.2:18080 -D HONCH_SANDBOX_API_KEY=honch_e2e_test_key build",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("idf.py log missing %q:\n%s", want, log)
		}
	}
}

func TestCCoreRunnerBuildUsesConfiguredHarnessDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake cmake script is POSIX-only")
	}
	repoRoot := t.TempDir()
	stateDir := filepath.Join(repoRoot, ".honch-sandbox")
	binDir := filepath.Join(repoRoot, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(repoRoot, "cmake.log")
	cmake := filepath.Join(binDir, "cmake")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + logPath + "\nexit 0\n"
	if err := os.WriteFile(cmake, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	r := CCoreRunner{
		RepoRoot:   repoRoot,
		StateDir:   stateDir,
		HarnessDir: "harnesses/custom-c-core",
	}
	if _, err := r.Build(context.Background()); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	wantSource := filepath.Join(repoRoot, "harnesses", "custom-c-core")
	if !strings.Contains(string(data), "-S "+wantSource) {
		t.Fatalf("cmake log did not use configured harness dir:\n%s", string(data))
	}
}

func TestEspIDFRunnerBuildUsesManagedIDFExportWhenIDFPathIsSet(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake export.sh script is POSIX-only")
	}
	repoRoot := t.TempDir()
	projectDir := filepath.Join(repoRoot, "harnesses", "esp-idf")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	idfPath := filepath.Join(repoRoot, "managed-idf")
	if err := os.MkdirAll(idfPath, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(repoRoot, "idf.log")
	exportScript := "idf.py() { printf '%s\\n' \"$PWD|$*\" >> " + logPath + "; }\nexport -f idf.py\n"
	if err := os.WriteFile(filepath.Join(idfPath, "export.sh"), []byte(exportScript), 0o700); err != nil {
		t.Fatal(err)
	}

	r := EspIDFRunner{RepoRoot: repoRoot, StateDir: filepath.Join(repoRoot, ".honch-sandbox"), IDFPath: idfPath}
	if _, err := r.Build(context.Background(), EspIDFSettings{
		Endpoint: "http://10.0.2.2:18080",
		Token:    "honch_e2e_test_key",
	}); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "set-target esp32") || !strings.Contains(string(data), "build") {
		t.Fatalf("managed idf export did not run idf.py commands:\n%s", string(data))
	}
}

func TestEspIDFRunnerRunStartsQEMUAndConnectsSerial(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake idf.py script is POSIX-only")
	}
	repoRoot := t.TempDir()
	projectDir := filepath.Join(repoRoot, "harnesses", "esp-idf")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(repoRoot, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(repoRoot, "runner.log")
	python := filepath.Join(binDir, "python")
	pythonScript := "#!/bin/sh\nprintf 'python|%s|%s\\n' \"$PWD\" \"$*\" >> " + logPath + "\n"
	if err := os.WriteFile(python, []byte(pythonScript), 0o700); err != nil {
		t.Fatal(err)
	}
	qemu := filepath.Join(binDir, "qemu-system-xtensa")
	qemuScript := "#!/bin/sh\nprintf 'qemu|%s|%s\\n' \"$PWD\" \"$*\" >> " + logPath + "\nsleep 2\n"
	if err := os.WriteFile(qemu, []byte(qemuScript), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HONCH_SANDBOX_QEMU_SERIAL_ADDR", "127.0.0.1:5555")
	stubQEMUSerial(t, []string{"ready\n"}, 3*time.Second)

	r := EspIDFRunner{RepoRoot: repoRoot, StateDir: filepath.Join(repoRoot, ".honch-sandbox")}
	build := EspIDFBuild{
		ProjectDir: projectDir,
		BuildDir:   filepath.Join(repoRoot, ".honch-sandbox", "build", "esp-idf"),
	}
	if err := os.MkdirAll(build.BuildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := r.Run(context.Background(), build, "", nil, nil); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	for _, want := range []string{
		"python|" + build.BuildDir + "|-m esptool --chip=esp32 merge-bin --output=" + filepath.Join(build.BuildDir, "qemu_flash.bin") + " --pad-to-size=2MB @flash_args",
		"qemu|",
		"|-M esp32 -m 4M",
		"-serial tcp:127.0.0.1:5555,server,nowait",
		"-nic user,model=open_eth",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("runner log missing %q:\n%s", want, log)
		}
	}
}

func TestEspIDFRunnerRunUsesConfiguredAdapterSettings(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake idf.py/qemu scripts are POSIX-only")
	}
	repoRoot := t.TempDir()
	projectDir := filepath.Join(repoRoot, "harnesses", "custom-esp-idf")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(repoRoot, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(repoRoot, "runner.log")
	idfPy := filepath.Join(binDir, "idf.py")
	idfScript := "#!/bin/sh\nprintf 'idf|%s|%s\\n' \"$PWD\" \"$*\" >> " + logPath + "\nexit 0\n"
	if err := os.WriteFile(idfPy, []byte(idfScript), 0o700); err != nil {
		t.Fatal(err)
	}
	python := filepath.Join(binDir, "python")
	if err := os.WriteFile(python, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	qemu := filepath.Join(binDir, "custom-qemu")
	qemuScript := "#!/bin/sh\nprintf 'qemu|%s|%s\\n' \"$PWD\" \"$*\" >> " + logPath + "\nsleep 2\n"
	if err := os.WriteFile(qemu, []byte(qemuScript), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HONCH_SANDBOX_QEMU_SERIAL_ADDR", "127.0.0.1:5555")
	stubQEMUSerial(t, []string{"ready\n"}, 3*time.Second)

	r := EspIDFRunner{
		RepoRoot:        repoRoot,
		StateDir:        filepath.Join(repoRoot, ".honch-sandbox"),
		HarnessDir:      "harnesses/custom-esp-idf",
		Target:          "esp32c3",
		RunTool:         "custom-qemu",
		EmulatorMachine: "esp32c3",
		EmulatorNetwork: "user,model=virtio-net",
	}
	build, err := r.Build(context.Background(), EspIDFSettings{
		Endpoint: "http://10.0.2.2:18080",
		Token:    "honch_e2e_test_key",
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if err := r.Run(context.Background(), build, "", io.Discard, io.Discard); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	for _, want := range []string{
		"idf|" + projectDir + "|-B " + filepath.Join(r.StateDir, "build", "esp-idf") + " -D SDKCONFIG=" + filepath.Join(r.StateDir, "build", "esp-idf", "sdkconfig") + " -D HONCH_SANDBOX_HOST=http://10.0.2.2:18080 -D HONCH_SANDBOX_API_KEY=honch_e2e_test_key set-target esp32c3",
		"custom-esp-idf",
		"-M esp32c3",
		"-nic user,model=virtio-net",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("runner log missing %q:\n%s", want, log)
		}
	}
}

func TestEspIDFRunnerRunFailsWhenSerialClosesAfterReady(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake qemu script is POSIX-only")
	}
	repoRoot := t.TempDir()
	projectDir := filepath.Join(repoRoot, "harnesses", "esp-idf")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(repoRoot, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	python := filepath.Join(binDir, "python")
	if err := os.WriteFile(python, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	qemuPIDPath := filepath.Join(repoRoot, "qemu.pid")
	qemu := filepath.Join(binDir, "qemu-system-xtensa")
	if err := os.WriteFile(qemu, []byte("#!/bin/sh\necho $$ > "+qemuPIDPath+"\nsleep 2\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HONCH_SANDBOX_QEMU_SERIAL_ADDR", "127.0.0.1:5555")
	stubQEMUSerial(t, []string{"ready\n"}, 1500*time.Millisecond)

	r := EspIDFRunner{RepoRoot: repoRoot, StateDir: filepath.Join(repoRoot, ".honch-sandbox")}
	build := EspIDFBuild{
		ProjectDir: projectDir,
		BuildDir:   filepath.Join(repoRoot, ".honch-sandbox", "build", "esp-idf"),
	}
	if err := os.MkdirAll(build.BuildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	err := r.Run(context.Background(), build, "", io.Discard, io.Discard)
	if err == nil {
		t.Fatal("Run succeeded after serial closed post-ready")
	}
	if !strings.Contains(err.Error(), "serial closed after firmware ready") {
		t.Fatalf("error did not explain post-ready serial close: %v", err)
	}
	pid := eventuallyReadPID(t, time.Second, qemuPIDPath)
	eventuallyNoProcess(t, time.Second, pid)
}

func TestEspIDFRunnerRunFailsWhenFirmwareNeverReportsReady(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake qemu script is POSIX-only")
	}
	repoRoot := t.TempDir()
	projectDir := filepath.Join(repoRoot, "harnesses", "esp-idf")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(repoRoot, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	python := filepath.Join(binDir, "python")
	if err := os.WriteFile(python, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	qemuPIDPath := filepath.Join(repoRoot, "qemu.pid")
	qemu := filepath.Join(binDir, "qemu-system-xtensa")
	if err := os.WriteFile(qemu, []byte("#!/bin/sh\necho $$ > "+qemuPIDPath+"\nsleep 2\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HONCH_SANDBOX_QEMU_READY_TIMEOUT", "1s")
	t.Setenv("HONCH_SANDBOX_QEMU_SERIAL_ADDR", "127.0.0.1:5555")
	stubQEMUSerial(t, []string{"booting\n"}, 2*time.Second)

	r := EspIDFRunner{RepoRoot: repoRoot, StateDir: filepath.Join(repoRoot, ".honch-sandbox")}
	build := EspIDFBuild{
		ProjectDir: projectDir,
		BuildDir:   filepath.Join(repoRoot, ".honch-sandbox", "build", "esp-idf"),
	}
	if err := os.MkdirAll(build.BuildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	err := r.Run(context.Background(), build, "", io.Discard, io.Discard)
	if err == nil {
		t.Fatal("Run succeeded without firmware ready marker")
	}
	if !strings.Contains(err.Error(), "firmware did not report ready") {
		t.Fatalf("error did not explain missing ready marker: %v", err)
	}
	pid := eventuallyReadPID(t, time.Second, qemuPIDPath)
	eventuallyNoProcess(t, time.Second, pid)
}

func TestKillAndWaitReapsStartedProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test is POSIX-only")
	}
	cmd := exec.Command("sh", "-c", "sleep 30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	killAndWait(cmd)
	if cmd.ProcessState == nil {
		t.Fatal("killAndWait did not wait for the process")
	}
}

func eventuallyReadPID(t *testing.T, timeout time.Duration, path string) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			return pid
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("pid file %s was not written", path)
	return 0
}

func eventuallyNoProcess(t *testing.T, timeout time.Duration, pid int) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if exec.Command("kill", "-0", strconv.Itoa(pid)).Run() != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %d was still alive after %s", pid, timeout)
}

func TestEspIDFRunnerBuildReportsMissingIDFPy(t *testing.T) {
	r := EspIDFRunner{RepoRoot: t.TempDir(), StateDir: t.TempDir()}

	_, err := r.Build(context.Background(), EspIDFSettings{
		Endpoint: "http://10.0.2.2:18080",
		Token:    "honch_e2e_test_key",
	})
	if err == nil {
		t.Fatal("Build succeeded without idf.py")
	}
	if !strings.Contains(err.Error(), "idf.py failed") {
		t.Fatalf("error did not name idf.py: %v", err)
	}
}
