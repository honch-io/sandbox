package commands_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	commands "honch.dev/honch/internal/commands"
)

type Dependencies = commands.Dependencies

func NewRootCommand(deps Dependencies) *cobra.Command {
	return commands.NewRootCommand(deps)
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
