package note

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shichao402/pkv/internal/bw/types"
	"github.com/shichao402/pkv/internal/state"
)

func TestSyncFolderCreatesAndTracksFiles(t *testing.T) {
	st := &state.State{}
	syncer := NewSyncer(st)
	dir := t.TempDir()

	items := []types.Item{{ID: "item1", Name: "config.yml", Notes: "key: value\n"}}
	count, err := syncer.SyncFolder(items, dir, "team-a")
	if err != nil {
		t.Fatalf("SyncFolder() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("SyncFolder() count = %d, want 1", count)
	}

	data, err := os.ReadFile(filepath.Join(dir, "config.yml"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "key: value\n" {
		t.Fatalf("file content = %q", string(data))
	}
	if len(st.Notes) != 1 {
		t.Fatalf("state notes = %d, want 1", len(st.Notes))
	}
	if st.Notes[0].TargetDir == "" {
		t.Fatal("target dir should be recorded")
	}
	if st.Notes[0].ContentHash == "" {
		t.Fatal("content hash should be recorded")
	}
}

func TestSyncFolderCreatesNestedFiles(t *testing.T) {
	st := &state.State{}
	syncer := NewSyncer(st)
	dir := t.TempDir()

	items := []types.Item{{ID: "item1", Name: "lyra/test/note", Notes: "nested\n"}}
	count, err := syncer.SyncFolder(items, dir, "team-a")
	if err != nil {
		t.Fatalf("SyncFolder() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("SyncFolder() count = %d, want 1", count)
	}

	path := filepath.Join(dir, "lyra", "test", "note")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read nested file: %v", err)
	}
	if string(data) != "nested\n" {
		t.Fatalf("file content = %q", string(data))
	}
	if st.Notes[0].FilePath != path {
		t.Fatalf("tracked file path = %q, want %q", st.Notes[0].FilePath, path)
	}
}

func TestSyncFolderUpdatesRenamedRemoteNote(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.env")
	if err := os.WriteFile(oldPath, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	st := &state.State{}
	absDir, _ := filepath.Abs(dir)
	st.AddNote(state.NoteEntry{
		ItemID:      "item1",
		Folder:      "team-a",
		TargetDir:   absDir,
		FileName:    "old.env",
		FilePath:    oldPath,
		ContentHash: hashContent("A=1\n"),
	})

	syncer := NewSyncer(st)
	items := []types.Item{{ID: "item1", Name: "new.env", Notes: "A=2\n"}}
	_, err := syncer.SyncFolder(items, dir, "team-a")
	if err != nil {
		t.Fatalf("SyncFolder() error = %v", err)
	}

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old path still exists")
	}
	newPath := filepath.Join(dir, "new.env")
	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read new file: %v", err)
	}
	if string(data) != "A=2\n" {
		t.Fatalf("new file content = %q", string(data))
	}
	entry := st.FindNoteEntry("item1", "team-a", absDir)
	if entry == nil {
		t.Fatal("expected tracked entry")
	}
	if entry.FileName != "new.env" {
		t.Fatalf("file name = %q", entry.FileName)
	}
}

func TestSyncFolderRemovesDeletedRemoteNote(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stale.txt")
	if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}

	absDir, _ := filepath.Abs(dir)
	st := &state.State{Notes: []state.NoteEntry{{
		ItemID:      "item1",
		Folder:      "team-a",
		TargetDir:   absDir,
		FileName:    "stale.txt",
		FilePath:    path,
		ContentHash: hashContent("stale"),
	}}}

	syncer := NewSyncer(st)
	count, err := syncer.SyncFolder(nil, dir, "team-a")
	if err != nil {
		t.Fatalf("SyncFolder() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("stale file should be removed")
	}
	if len(st.Notes) != 0 {
		t.Fatalf("state notes = %d, want 0", len(st.Notes))
	}
}

func TestSyncFolderKeepsLocallyModifiedDeletedRemoteNote(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stale.txt")
	if err := os.WriteFile(path, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}

	absDir, _ := filepath.Abs(dir)
	st := &state.State{Notes: []state.NoteEntry{{
		ItemID:      "item1",
		Folder:      "team-a",
		TargetDir:   absDir,
		FileName:    "stale.txt",
		FilePath:    path,
		ContentHash: hashContent("original"),
	}}}

	syncer := NewSyncer(st)
	_, err := syncer.SyncFolder(nil, dir, "team-a")
	if err == nil {
		t.Fatal("expected modified stale note error")
	}
	if len(st.Notes) != 1 {
		t.Fatalf("state notes = %d, want 1", len(st.Notes))
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("local file should remain: %v", err)
	}
}

func TestSyncFolderFailsOnUntrackedFileConflict(t *testing.T) {
	dir := t.TempDir()
	conflict := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(conflict, []byte("manual"), 0o600); err != nil {
		t.Fatal(err)
	}

	syncer := NewSyncer(&state.State{})
	_, err := syncer.SyncFolder([]types.Item{{ID: "item1", Name: "config.yml", Notes: "remote"}}, dir, "team-a")
	if err == nil {
		t.Fatal("expected conflict error")
	}
}

func TestSyncFolderRejectsEscapingPath(t *testing.T) {
	dir := t.TempDir()
	syncer := NewSyncer(&state.State{})

	_, err := syncer.SyncFolder([]types.Item{{ID: "item1", Name: "../config.yml", Notes: "remote"}}, dir, "team-a")
	if err == nil {
		t.Fatal("expected escaping path error")
	}
}

func TestRemoveDeletesEmptyParentDirsWithinTarget(t *testing.T) {
	st := &state.State{}
	syncer := NewSyncer(st)
	dir := t.TempDir()
	filePath := filepath.Join(dir, "lyra", "test", "note")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}

	entry := state.NoteEntry{
		ItemID:    "item1",
		FileName:  "lyra/test/note",
		FilePath:  filePath,
		TargetDir: dir,
	}

	if err := syncer.Remove(entry); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "lyra")); !os.IsNotExist(err) {
		t.Fatalf("expected empty parent dirs to be removed, stat err = %v", err)
	}
}

func TestRemoveKeepsTargetDir(t *testing.T) {
	st := &state.State{}
	syncer := NewSyncer(st)
	dir := t.TempDir()
	filePath := filepath.Join(dir, "note")
	if err := os.WriteFile(filePath, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}

	entry := state.NoteEntry{
		ItemID:    "item1",
		FileName:  "note",
		FilePath:  filePath,
		TargetDir: dir,
	}

	if err := syncer.Remove(entry); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("target dir should remain: %v", err)
	}
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
