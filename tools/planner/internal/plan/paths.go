package plan

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandHome replaces a leading ~ or ~/ with the home directory.
func ExpandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, path[1:]), nil
}
