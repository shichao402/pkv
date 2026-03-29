package note

import (
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

// Sync writes a note's content to a file in the target directory.
// Returns an error if the file already exists (to prevent silent overwrite).
func (s *Syncer) Sync(item types.Item, targetDir string) error {
	if item.Notes == "" {
		return fmt.Errorf("item '%s' has no note content", item.Name)
	}

	filePath := filepath.Join(targetDir, item.Name)

	// Check if file already exists to prevent silent overwrite
	if _, err := os.Stat(filePath); err == nil {
		// File exists, return error instead of silently overwriting
		return fmt.Errorf("file '%s' already exists (remove it first or update manually)", item.Name)
	} else if !os.IsNotExist(err) {
		// Some other error occurred during stat
		return fmt.Errorf("check file status: %w", err)
	}

	if err := os.WriteFile(filePath, []byte(item.Notes), 0o600); err != nil {
		return fmt.Errorf("write note file: %w", err)
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}

	s.state.AddNote(state.NoteEntry{
		ItemID:   item.ID,
		FileName: item.Name,
		FilePath: absPath,
	})

	return nil
}

// Remove deletes a previously synced note file.
func (s *Syncer) Remove(entry state.NoteEntry) error {
	if _, err := os.Stat(entry.FilePath); os.IsNotExist(err) {
		return nil // Already gone
	}
	return os.Remove(entry.FilePath)
}
