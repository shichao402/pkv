package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var conflictNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// ConflictsDir returns the workspace-local directory for conflict copies.
func ConflictsDir(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, ".pkv", "conflicts")
}

// ConflictCopyName builds a deterministic conflict filename.
func ConflictCopyName(itemID, side, fileName string, at time.Time) string {
	base := conflictNameSanitizer.ReplaceAllString(filepath.Base(fileName), "_")
	if base == "" || base == "." {
		base = "note"
	}
	ts := at.UTC().Format("20060102T150405Z")
	return fmt.Sprintf("%s_%s_%s_%s", itemID, side, ts, base)
}

// WriteConflictCopy stores a losing-side note body under .pkv/conflicts/.
func WriteConflictCopy(workspaceRoot, itemID, side, fileName string, content []byte) (string, error) {
	dir := ConflictsDir(workspaceRoot)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := ConflictCopyName(itemID, side, fileName, time.Now().UTC())
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// ListConflictFiles returns conflict copy paths under a workspace root.
func ListConflictFiles(workspaceRoot string) ([]string, error) {
	dir := ConflictsDir(workspaceRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	return paths, nil
}

// ParseConflictFileName extracts itemID and side from a conflict copy filename.
func ParseConflictFileName(name string) (itemID, side string, ok bool) {
	parts := strings.SplitN(name, "_", 4)
	if len(parts) < 4 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// DeleteConflictCopies removes conflict copies for itemID under a workspace root.
func DeleteConflictCopies(workspaceRoot, itemID string) ([]string, error) {
	paths, err := ListConflictFiles(workspaceRoot)
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, path := range paths {
		item, _, ok := ParseConflictFileName(filepath.Base(path))
		if !ok || item != itemID {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return removed, err
		}
		removed = append(removed, path)
	}
	return removed, nil
}
