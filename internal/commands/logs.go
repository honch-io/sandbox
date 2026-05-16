package commands

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/honch/sdk/tools/sandbox/internal/config"
	"github.com/honch/sdk/tools/sandbox/internal/ui"
	"github.com/spf13/cobra"
)

type logOptions struct {
	Tail int
}

func newLogsCommand(deps Dependencies) *cobra.Command {
	var tail int
	cmd := &cobra.Command{
		Use:   "logs [stack|device|proxy]",
		Short: "Print recent sandbox logs",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return errors.New(ui.FormatError("too many log targets", []ui.Row{
					{Key: "required", Value: "honch sandbox logs [stack|device|proxy]"},
					{Key: "example", Value: "honch sandbox logs device"},
				}))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, _, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			target := "stack"
			if len(args) == 1 {
				target = args[0]
			}
			return printLogs(cmd.OutOrStdout(), root, cfg, target, logOptions{Tail: tail})
		},
	}
	cmd.Flags().IntVar(&tail, "tail", 80, "number of recent log lines to print per file")
	return cmd
}

func printLogs(out io.Writer, root string, cfg config.Config, target string, opts logOptions) error {
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
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				_, _ = fmt.Fprintf(out, "%s: no logs yet\n", file)
				continue
			}
			return err
		}
		tail := opts.Tail
		if tail <= 0 {
			tail = 80
		}
		text, err := tailFile(path, tail)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "\n%s\n", ui.Heading(file))
		_, _ = fmt.Fprintf(out, "path: %s\n", path)
		_, _ = fmt.Fprintf(out, "showing last %d lines\n\n", tail)
		_, _ = fmt.Fprint(out, text)
		if len(text) > 0 && text[len(text)-1] != '\n' {
			_, _ = fmt.Fprintln(out)
		}
	}
	return nil
}

func tailFile(path string, maxLines int) (string, error) {
	if maxLines <= 0 {
		maxLines = 80
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	lines := make([]string, maxLines)
	count := 0
	for {
		line, readErr := reader.ReadString('\n')
		if line != "" {
			lines[count%maxLines] = line
			count++
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return "", readErr
		}
	}
	if count == 0 {
		return "", nil
	}
	kept := count
	start := 0
	if count > maxLines {
		kept = maxLines
		start = count % maxLines
	}
	var result strings.Builder
	for i := 0; i < kept; i++ {
		result.WriteString(lines[(start+i)%maxLines])
	}
	return result.String(), nil
}
