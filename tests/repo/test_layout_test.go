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
