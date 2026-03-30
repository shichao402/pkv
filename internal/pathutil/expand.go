package pathutil

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandTilde expands a leading ~ in path to the current user's home directory.
// Paths without a leading ~ are returned unchanged.
func ExpandTilde(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, path[1:]), nil
}
