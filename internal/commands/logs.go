package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/honch/sdk/tools/sandbox/internal/config"
	"github.com/honch/sdk/tools/sandbox/internal/ui"
)

func printLogs(out io.Writer, root string, cfg config.Config, target string) error {
	logDir := filepath.Join(root, cfg.Sandbox.StateDir, "logs")
	var files []string
	switch target {
	case "stack":
		files = []string{"capture.log", "worker.log"}
	case "device":
		files = []string{"device.log"}
	case "proxy":
		files = []string{"proxy.log"}
	default:
		return errors.New(ui.FormatError(fmt.Sprintf("unknown log target %q", target), []ui.Row{
			{Key: "required", Value: "stack, device, or proxy"},
			{Key: "example", Value: "honch sandbox logs device"},
		}))
	}
	for _, file := range files {
		path := filepath.Join(logDir, file)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				_, _ = fmt.Fprintf(out, "%s: no logs yet\n", file)
				continue
			}
			return err
		}
		_, _ = fmt.Fprintf(out, "\n%s\n", ui.Heading(file))
		_, _ = fmt.Fprint(out, tailString(string(data), 80))
		if len(data) > 0 && data[len(data)-1] != '\n' {
			_, _ = fmt.Fprintln(out)
		}
	}
	return nil
}

func tailString(text string, maxLines int) string {
	lines := strings.SplitAfter(text, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "")
}
