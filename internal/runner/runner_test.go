package runner

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

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

func TestEspIDFRunnerBuildUsesIDFPyWithStateBuildDirAndSandboxDefines(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake idf.py script is POSIX-only")
	}
	repoRoot := t.TempDir()
	stateDir := filepath.Join(repoRoot, ".honch-sandbox")
	projectDir := filepath.Join(repoRoot, "tools", "sandbox", "harnesses", "esp-idf")
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

func TestEspIDFRunnerBuildUsesManagedIDFExportWhenIDFPathIsSet(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake export.sh script is POSIX-only")
	}
	repoRoot := t.TempDir()
	projectDir := filepath.Join(repoRoot, "tools", "sandbox", "harnesses", "esp-idf")
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
	projectDir := filepath.Join(repoRoot, "tools", "sandbox", "harnesses", "esp-idf")
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
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	t.Setenv("HONCH_SANDBOX_QEMU_SERIAL_ADDR", listener.Addr().String())
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		_, _ = conn.Write([]byte("ready\n"))
		time.Sleep(500 * time.Millisecond)
		_ = conn.Close()
	}()

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
		"-serial tcp::",
		"-nic user,model=open_eth",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("runner log missing %q:\n%s", want, log)
		}
	}
}

func TestEspIDFRunnerRunFailsWhenFirmwareNeverReportsReady(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake qemu script is POSIX-only")
	}
	repoRoot := t.TempDir()
	projectDir := filepath.Join(repoRoot, "tools", "sandbox", "harnesses", "esp-idf")
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
	qemu := filepath.Join(binDir, "qemu-system-xtensa")
	if err := os.WriteFile(qemu, []byte("#!/bin/sh\nsleep 2\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HONCH_SANDBOX_QEMU_READY_TIMEOUT", "200ms")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	t.Setenv("HONCH_SANDBOX_QEMU_SERIAL_ADDR", listener.Addr().String())
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte("booting\n"))
		time.Sleep(time.Second)
	}()

	r := EspIDFRunner{RepoRoot: repoRoot, StateDir: filepath.Join(repoRoot, ".honch-sandbox")}
	build := EspIDFBuild{
		ProjectDir: projectDir,
		BuildDir:   filepath.Join(repoRoot, ".honch-sandbox", "build", "esp-idf"),
	}
	if err := os.MkdirAll(build.BuildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	err = r.Run(context.Background(), build, "", io.Discard, io.Discard)
	if err == nil {
		t.Fatal("Run succeeded without firmware ready marker")
	}
	if !strings.Contains(err.Error(), "firmware did not report ready") {
		t.Fatalf("error did not explain missing ready marker: %v", err)
	}
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
