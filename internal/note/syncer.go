package note

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shichao402/pkv/internal/bw/types"
	"github.com/shichao402/pkv/internal/state"
)

type Syncer struct {
	state *state.State
}

func NewSyncer(st *state.State) *Syncer {
	return &Syncer{state: st}
}

// SyncFolder reconciles all config notes from a folder into the target directory.
// Existing tracked files are updated in place, remote renames are reflected locally,
// and deleted remote notes are removed from the target directory.
func (s *Syncer) SyncFolder(items []types.Item, targetDir, folder string) (int, error) {
	absTargetDir, err := filepath.Abs(targetDir)
	if err == nil {
		targetDir = absTargetDir
	}

	tracked := s.state.FindSyncedNotes(folder, targetDir)
	trackedByID := make(map[string]state.NoteEntry, len(tracked))
	for _, entry := range tracked {
		trackedByID[entry.ItemID] = entry
	}

	remoteByID := make(map[string]types.Item, len(items))
	for _, item := range items {
		remoteByID[item.ID] = item
	}

	for _, entry := range tracked {
		if _, ok := remoteByID[entry.ItemID]; ok {
			continue
		}
		localHash, hasLocalFile, err := currentFileHash(entry.FilePath)
		if err != nil {
			return 0, fmt.Errorf("read local stale note '%s': %w", entry.FileName, err)
		}
		if hasLocalFile && entry.ContentHash != "" && localHash != entry.ContentHash {
			return 0, fmt.Errorf("local note '%s' was modified after last sync; refusing to remove it because the remote note is gone", entry.FileName)
		}
		if err := s.Remove(entry); err != nil {
			return 0, fmt.Errorf("remove stale note '%s': %w", entry.FileName, err)
		}
		s.state.RemoveNoteForTarget(entry.ItemID, folder, targetDir)
	}

	synced := 0
	for _, item := range items {
		if item.Notes == "" {
			return synced, fmt.Errorf("item '%s' has no note content", item.Name)
		}

		entry, exists := trackedByID[item.ID]
		if exists {
			if err := s.updateTracked(item, entry, targetDir, folder); err != nil {
				return synced, err
			}
			synced++
			continue
		}

		if err := s.createNew(item, targetDir, folder); err != nil {
			return synced, err
		}
		synced++
	}

	return synced, nil
}

func (s *Syncer) createNew(item types.Item, targetDir, folder string) error {
	filePath := filepath.Join(targetDir, item.Name)
	if err := ensureWritableNewFile(filePath); err != nil {
		return fmt.Errorf("prepare new note '%s': %w", item.Name, err)
	}
	if err := writeNoteFile(filePath, item.Notes); err != nil {
		return fmt.Errorf("write note '%s': %w", item.Name, err)
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}
	absTargetDir, err := filepath.Abs(targetDir)
	if err != nil {
		absTargetDir = targetDir
	}

	s.state.AddNote(state.NoteEntry{
		ItemID:      item.ID,
		Folder:      folder,
		TargetDir:   absTargetDir,
		FileName:    item.Name,
		FilePath:    absPath,
		ContentHash: hashContent(item.Notes),
	})
	return nil
}

func (s *Syncer) updateTracked(item types.Item, entry state.NoteEntry, targetDir, folder string) error {
	newPath := filepath.Join(targetDir, item.Name)
	absNewPath, err := filepath.Abs(newPath)
	if err == nil {
		newPath = absNewPath
	}
	absTargetDir, err := filepath.Abs(targetDir)
	if err == nil {
		targetDir = absTargetDir
	}

	localHash, hasLocalFile, err := currentFileHash(entry.FilePath)
	if err != nil {
		return fmt.Errorf("read local note '%s': %w", entry.FileName, err)
	}
	if hasLocalFile && entry.ContentHash != "" && localHash != entry.ContentHash {
		return fmt.Errorf("local note '%s' was modified; use 'pkv edit %s note %s' or remove the local file before syncing", entry.FileName, folder, entry.FileName)
	}

	if entry.FilePath != newPath {
		if err := renameTrackedFile(entry.FilePath, newPath); err != nil {
			return fmt.Errorf("rename tracked note '%s': %w", entry.FileName, err)
		}
	}

	contentHash := hashContent(item.Notes)
	if !hasLocalFile || entry.ContentHash != contentHash || entry.FilePath != newPath || entry.FileName != item.Name {
		if err := writeNoteFile(newPath, item.Notes); err != nil {
			return fmt.Errorf("update note '%s': %w", item.Name, err)
		}
	}

	s.state.AddNote(state.NoteEntry{
		ItemID:      item.ID,
		Folder:      folder,
		TargetDir:   targetDir,
		FileName:    item.Name,
		FilePath:    newPath,
		ContentHash: contentHash,
	})
	return nil
}

func ensureWritableNewFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("file already exists: %s", filepath.Base(path))
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func renameTrackedFile(oldPath, newPath string) error {
	if oldPath == newPath {
		return nil
	}
	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("target file already exists: %s", filepath.Base(newPath))
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return nil
}

func writeNoteFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}

func hashContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func currentFileHash(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return hashContent(string(data)), true, nil
}

// Remove deletes a previously synced note file.
func (s *Syncer) Remove(entry state.NoteEntry) error {
	if _, err := os.Stat(entry.FilePath); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(entry.FilePath)
}
