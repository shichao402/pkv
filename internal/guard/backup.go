package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// maxBackupsPerNote is the maximum backup files kept per Bitwarden note item.
	maxBackupsPerNote = 30
	backupRootName    = ".pkv/backups"
)

// BackupNoteContent stores content under ~/.pkv/backups/<item_id>/.
// label distinguishes local vs remote snapshots (e.g. "local", "remote").
// When a note already has maxBackupsPerNote files, the oldest is deleted.
func BackupNoteContent(itemID, label, fileName string, content []byte) (string, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return "", fmt.Errorf("item_id is required for backup")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, backupRootName, sanitizeBackupSegment(itemID))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}

	ts := fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	name := sanitizeBackupSegment(fileName)
	if name == "" {
		name = "note"
	}
	label = sanitizeBackupSegment(label)
	if label == "" {
		label = "snapshot"
	}
	backupPath := filepath.Join(dir, fmt.Sprintf("%s_%s_%s", ts, label, name))
	if err := os.WriteFile(backupPath, content, 0o600); err != nil {
		return "", err
	}
	if err := rotateBackups(dir, maxBackupsPerNote); err != nil {
		return backupPath, err
	}
	return backupPath, nil
}

func sanitizeBackupSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_")
	return replacer.Replace(value)
}

func rotateBackups(dir string, maxKeep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) <= maxKeep {
		return nil
	}
	type named struct {
		name string
		path string
	}
	files := make([]named, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		files = append(files, named{name: entry.Name(), path: filepath.Join(dir, entry.Name())})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].name < files[j].name
	})
	for len(files) > maxKeep {
		if err := os.Remove(files[0].path); err != nil && !os.IsNotExist(err) {
			return err
		}
		files = files[1:]
	}
	return nil
}
