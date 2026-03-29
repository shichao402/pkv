package note

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shichao402/pkv/internal/bw/types"
	"github.com/shichao402/pkv/internal/state"
)

func TestSync(t *testing.T) {
	t.Run("normal sync creates file", func(t *testing.T) {
		st := &state.State{}
		syncer := NewSyncer(st)
		dir := t.TempDir()

		item := types.Item{
			ID:    "item1",
			Name:  "config.yml",
			Notes: "key: value\n",
		}

		if err := syncer.Sync(item, dir); err != nil {
			t.Fatalf("Sync() error = %v", err)
		}

		filePath := filepath.Join(dir, "config.yml")
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("failed to read synced file: %v", err)
		}
		if string(data) != "key: value\n" {
			t.Errorf("file content = %q, want %q", string(data), "key: value\n")
		}
	})

	t.Run("empty notes returns error", func(t *testing.T) {
		st := &state.State{}
		syncer := NewSyncer(st)
		dir := t.TempDir()

		item := types.Item{
			ID:    "item1",
			Name:  "empty.txt",
			Notes: "",
		}

		err := syncer.Sync(item, dir)
		if err == nil {
			t.Fatal("Sync() expected error for empty notes, got nil")
		}
	})

	t.Run("file already exists returns error", func(t *testing.T) {
		st := &state.State{}
		syncer := NewSyncer(st)
		dir := t.TempDir()

		// Pre-create the file
		existingPath := filepath.Join(dir, "exists.txt")
		if err := os.WriteFile(existingPath, []byte("existing"), 0o600); err != nil {
			t.Fatalf("failed to create existing file: %v", err)
		}

		item := types.Item{
			ID:    "item1",
			Name:  "exists.txt",
			Notes: "new content",
		}

		err := syncer.Sync(item, dir)
		if err == nil {
			t.Fatal("Sync() expected error for existing file, got nil")
		}
	})

	t.Run("file permission is 0600", func(t *testing.T) {
		st := &state.State{}
		syncer := NewSyncer(st)
		dir := t.TempDir()

		item := types.Item{
			ID:    "item1",
			Name:  "secret.txt",
			Notes: "secret content",
		}

		if err := syncer.Sync(item, dir); err != nil {
			t.Fatalf("Sync() error = %v", err)
		}

		filePath := filepath.Join(dir, "secret.txt")
		info, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("failed to stat file: %v", err)
		}
		perm := info.Mode().Perm()
		if perm != 0o600 {
			t.Errorf("file permission = %o, want %o", perm, 0o600)
		}
	})

	t.Run("state records note correctly", func(t *testing.T) {
		st := &state.State{}
		syncer := NewSyncer(st)
		dir := t.TempDir()

		item := types.Item{
			ID:    "item1",
			Name:  "tracked.txt",
			Notes: "tracked content",
		}

		if err := syncer.Sync(item, dir); err != nil {
			t.Fatalf("Sync() error = %v", err)
		}

		if len(st.Notes) != 1 {
			t.Fatalf("expected 1 note in state, got %d", len(st.Notes))
		}
		entry := st.Notes[0]
		if entry.ItemID != "item1" {
			t.Errorf("state ItemID = %q, want %q", entry.ItemID, "item1")
		}
		if entry.FileName != "tracked.txt" {
			t.Errorf("state FileName = %q, want %q", entry.FileName, "tracked.txt")
		}
		if entry.FilePath == "" {
			t.Error("state FilePath should not be empty")
		}
		if entry.SyncedAt == "" {
			t.Error("state SyncedAt should not be empty")
		}
	})
}

func TestRemove(t *testing.T) {
	t.Run("file exists gets deleted", func(t *testing.T) {
		st := &state.State{}
		syncer := NewSyncer(st)
		dir := t.TempDir()

		filePath := filepath.Join(dir, "to_remove.txt")
		if err := os.WriteFile(filePath, []byte("content"), 0o600); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}

		entry := state.NoteEntry{
			ItemID:   "item1",
			FileName: "to_remove.txt",
			FilePath: filePath,
		}

		if err := syncer.Remove(entry); err != nil {
			t.Fatalf("Remove() error = %v", err)
		}

		if _, err := os.Stat(filePath); !os.IsNotExist(err) {
			t.Error("Remove() file should not exist after removal")
		}
	})

	t.Run("file not exists returns nil", func(t *testing.T) {
		st := &state.State{}
		syncer := NewSyncer(st)
		dir := t.TempDir()

		entry := state.NoteEntry{
			ItemID:   "item1",
			FileName: "nonexistent.txt",
			FilePath: filepath.Join(dir, "nonexistent.txt"),
		}

		err := syncer.Remove(entry)
		if err != nil {
			t.Errorf("Remove() error = %v, want nil for nonexistent file", err)
		}
	})
}
