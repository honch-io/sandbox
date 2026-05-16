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
	"github.com/honch/sdk/tools/sandbox/internal/session"
)

func loadRuntime(deps Dependencies) (string, config.Config, session.Manager, error) {
	root := deps.RootDir
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", config.Config{}, session.Manager{}, err
		}
		root = findRepoRoot(wd)
	}
	cfg, err := config.Load(root)
	if err != nil {
		return "", config.Config{}, session.Manager{}, err
	}
	manager := session.NewManager(filepath.Join(root, cfg.Sandbox.StateDir, "session.json"))
	return root, cfg, manager, nil
}

func findRepoRoot(start string) string {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, "c-core")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return start
		}
		dir = parent
	}
}

func boolCount(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}

func valueOr(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func confirm(in io.Reader, out io.Writer, prompt string) (bool, error) {
	_, _ = fmt.Fprint(out, prompt)
	reader := bufio.NewReader(in)
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}

func stringTrim(data []byte) string {
	for len(data) > 0 && (data[0] == '\n' || data[0] == '\r' || data[0] == ' ' || data[0] == '\t') {
		data = data[1:]
	}
	for len(data) > 0 {
		last := data[len(data)-1]
		if last != '\n' && last != '\r' && last != ' ' && last != '\t' {
			break
		}
		data = data[:len(data)-1]
	}
	return string(data)
}
