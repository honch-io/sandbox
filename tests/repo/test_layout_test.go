package repo_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTestsDirectoryDoesNotMirrorInternalPackages(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	mirroredInternal := filepath.Join(root, "tests", "internal")
	if _, err := os.Stat(mirroredInternal); os.IsNotExist(err) {
		return
	} else if err != nil {
		t.Fatal(err)
	}

	var mirrored []string
	err = filepath.WalkDir(mirroredInternal, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		mirrored = append(mirrored, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(mirrored) > 0 {
		t.Fatalf("tests/internal must not mirror production sources: %s", strings.Join(mirrored, ", "))
	}
}

func TestRepositoryDoesNotShipStaleStaticSite(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(root, "site")); err == nil {
		t.Fatal("site/ should not be present in the sandbox repository")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestCLIEntrypointLivesUnderCmdHonch(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(root, "main.go")); err == nil {
		t.Fatal("root main.go should not be present; use cmd/honch as the canonical CLI entrypoint")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "cmd", "honch", "main.go")); err != nil {
		t.Fatal(err)
	}
}

func TestSandboxHarnessesReferenceCanonicalSDKLayout(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	checks := []struct {
		path   string
		want   string
		legacy []string
	}{
		{
			path: "harnesses/c-core/CMakeLists.txt",
			want: "../../../SDK/ports/posix",
			legacy: []string{
				"../../../SDK/c-core",
				"honch_c_core",
			},
		},
		{
			path: "harnesses/esp-idf/CMakeLists.txt",
			want: "../../../SDK/ports/esp-idf/honch",
			legacy: []string{
				"../../../SDK/esp-idf/honch",
			},
		},
	}

	for _, check := range checks {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(check.path)))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.Contains(text, check.want) {
			t.Fatalf("%s does not reference canonical SDK path %q", check.path, check.want)
		}
		for _, legacy := range check.legacy {
			if strings.Contains(text, legacy) {
				t.Fatalf("%s still references legacy SDK path/target %q", check.path, legacy)
			}
		}
	}
}

func TestESPIDFHarnessEmitsBootSmokeEvent(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(root, "harnesses", "esp-idf", "main", "app_main.c"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `sandbox_app_track("sdk.esp32.boot"`) {
		t.Fatalf("esp-idf harness should emit a boot smoke event for hardware validation")
	}
	if !strings.Contains(text, "sandbox_app_flush()") {
		t.Fatalf("esp-idf harness should flush the boot smoke event for hardware validation")
	}
	if !strings.Contains(text, "esp_sntp_init()") {
		t.Fatalf("esp-idf hardware harness should sync time before emitting smoke events")
	}
}

func TestESPIDFHarnessCanTriggerRealPanicForFaultCapture(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(root, "harnesses", "esp-idf", "main", "sandbox_control.c"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `strcmp(action->valuestring, "panic") == 0`) {
		t.Fatalf("esp-idf harness should expose a panic control action for real fault-capture validation")
	}
	if !strings.Contains(text, "abort();") {
		t.Fatalf("esp-idf panic control action should trigger a real abort/panic reset")
	}
}
