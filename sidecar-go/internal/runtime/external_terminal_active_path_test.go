package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoExternalTerminalActivePathReferences(t *testing.T) {
	sidecarRoot := filepath.Clean(filepath.Join("..", ".."))
	forbidden := []string{
		"ter" + "mux-",
		"Ter" + "mux",
		"ter" + "mux",
	}

	if err := filepath.WalkDir(sidecarRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == "vendor" || name == ".xmilo" {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(sidecarRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if filepath.Ext(path) != ".go" && rel != "README.md" {
			return nil
		}
		if rel == "internal/runtime/external_terminal_active_path_test.go" {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(raw)
		for _, fragment := range forbidden {
			if strings.Contains(text, fragment) {
				t.Fatalf("forbidden external-terminal runtime reference found in %s", rel)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
