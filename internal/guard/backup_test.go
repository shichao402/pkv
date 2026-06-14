package guard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupNoteContentRotatesOldFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	itemID := "note-item-1"
	dir := filepath.Join(home, backupRootName, itemID)

	for i := 0; i < maxBackupsPerNote+5; i++ {
		if _, err := BackupNoteContent(itemID, "local", "demo.md", []byte("content")); err != nil {
			t.Fatalf("BackupNoteContent() error = %v", err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != maxBackupsPerNote {
		t.Fatalf("backup count = %d, want %d", len(entries), maxBackupsPerNote)
	}
}

func TestBackupNoteContentRequiresItemID(t *testing.T) {
	if _, err := BackupNoteContent("", "local", "demo.md", []byte("x")); err == nil {
		t.Fatal("expected error for empty item_id")
	}
}
