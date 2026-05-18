package repo_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoTestsLiveUnderTestsDirectory(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	var misplaced []string
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "build":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(filepath.ToSlash(rel), "tests/") {
			misplaced = append(misplaced, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(misplaced) > 0 {
		t.Fatalf("Go test files must live under tests/: %s", strings.Join(misplaced, ", "))
	}
}
