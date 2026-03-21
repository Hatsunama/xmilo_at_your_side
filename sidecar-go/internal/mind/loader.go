package mind

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Loader struct {
	Root string
}

func New(root string) Loader {
	return Loader{Root: root}
}

func (l Loader) SystemPrompt() (string, error) {
	parts := []string{}
	for _, name := range []string{"IDENTITY.md", "SOUL.md", "SECURITY.md", "TOOLS.md", "USER.md"} {
		data, err := os.ReadFile(filepath.Join(l.Root, name))
		if err != nil {
			return "", fmt.Errorf("read %s: %w", name, err)
		}
		parts = append(parts, string(data))
	}
	return strings.Join(parts, "\n\n---\n\n"), nil
}
