package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"honch.dev/honch/internal/proxy"
	"honch.dev/honch/internal/session"
)

func TestSandboxRunRejectsInactiveSandboxAndSkipsBuild(t *testing.T) {
	rootDir := t.TempDir()
	writeAdapterRegistryForTest(t, rootDir)
	binDir := filepath.Join(rootDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(rootDir, "cmake.called")
	cmake := filepath.Join(binDir, "cmake")
	script := "#!/bin/sh\necho \"$*\" > " + marker + "\nexit 99\n"
	if err := os.WriteFile(cmake, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	manager := session.NewManager(filepath.Join(rootDir, ".honch-sandbox", "session.json"))
	if err := manager.Save(session.State{
		Stack: session.StackState{Running: true},
		Proxy: session.ProxyState{Mode: proxy.ModeOffline.String(), Port: 18080},
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
		t.Fatal("run succeeded without a live sandbox")
	}
	combined := err.Error() + "\n" + out.String()
	if !strings.Contains(combined, "sandbox is not running") {
		t.Fatalf("run error did not explain the missing live sandbox:\n%s", combined)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("run reached the build step even though the sandbox was not live: %v", err)
	}
}

func TestSandboxRunHardwareReadsWiFiFromSDKLocalDefaults(t *testing.T) {
	rootDir := t.TempDir()
	sdkLocalDir := filepath.Clean(filepath.Join(rootDir, "..", "SDK", "ports", "esp-idf", "local"))
	if err := os.MkdirAll(sdkLocalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	localDefaults := strings.Join([]string{
		`CONFIG_WIFI_SSID="lab-network"`,
		`CONFIG_WIFI_PASSWORD="lab-password"`,
		`CONFIG_HONCH_API_KEY="local-api-key"`,
		`CONFIG_HONCH_HOST="http://192.168.1.122:8001"`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(sdkLocalDir, "sdkconfig.defaults"), []byte(localDefaults), 0o600); err != nil {
		t.Fatal(err)
	}

	ssid, password, ok := sdkLocalWiFiDefaults(rootDir)
	if !ok {
		t.Fatal("local SDK Wi-Fi defaults were not found")
	}
	if ssid != "lab-network" {
		t.Fatalf("ssid = %q", ssid)
	}
	if password != "lab-password" {
		t.Fatalf("password = %q", password)
	}
}
