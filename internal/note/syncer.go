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
func (s *Syncer) Sync(item types.Item, targetDir string) error {
	if item.Notes == "" {
		return fmt.Errorf("item '%s' has no note content", item.Name)
	}

	filePath := filepath.Join(targetDir, item.Name)
	if err := os.WriteFile(filePath, []byte(item.Notes), 0600); err != nil {
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
