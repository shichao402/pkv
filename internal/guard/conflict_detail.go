package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shichao402/pkv/internal/state"
)

const contentSummaryMaxLen = 256

type ConflictCopyView struct {
	Side    string `json:"side"`
	Path    string `json:"path"`
	Summary string `json:"summary"`
}

type ConflictDetail struct {
	ItemID           string             `json:"item_id"`
	FileName         string             `json:"file_name"`
	Folder           string             `json:"folder"`
	TargetDir        string             `json:"target_dir,omitempty"`
	CanonicalPath    string             `json:"canonical_path"`
	CanonicalSummary string             `json:"canonical_summary"`
	Copies           []ConflictCopyView `json:"copies"`
}

func SummarizeContent(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return "(empty)"
	}
	lines := strings.Split(content, "\n")
	if len(lines) > 5 {
		content = strings.Join(lines[:5], "\n") + "\n..."
	}
	runes := []rune(content)
	if len(runes) > contentSummaryMaxLen {
		return string(runes[:contentSummaryMaxLen]) + "..."
	}
	return content
}

func readContentSummary(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "(missing)"
		}
		return fmt.Sprintf("(read error: %v)", err)
	}
	return SummarizeContent(string(data))
}

// ShowConflict returns canonical and conflict-copy paths with content summaries.
func ShowConflict(st *state.State, itemID string) (ConflictDetail, error) {
	if st == nil {
		return ConflictDetail{}, fmt.Errorf("state is nil")
	}
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return ConflictDetail{}, fmt.Errorf("item_id is required")
	}

	var entry *state.NoteEntry
	for i := range st.Notes {
		if st.Notes[i].ItemID == itemID && st.Notes[i].Conflict != "" {
			entry = &st.Notes[i]
			break
		}
	}
	if entry == nil {
		return ConflictDetail{}, fmt.Errorf("no pending conflict for item_id: %s", itemID)
	}

	detail := ConflictDetail{
		ItemID:        entry.ItemID,
		FileName:      entry.FileName,
		Folder:        entry.Folder,
		TargetDir:     entry.TargetDir,
		CanonicalPath: entry.FilePath,
	}
	if entry.FilePath != "" {
		detail.CanonicalSummary = readContentSummary(entry.FilePath)
	}

	ws := findWorkspaceForNote(st, entry.Folder, entry.TargetDir, entry.TargetDir)
	if ws != nil {
		paths, err := ListConflictFiles(ws.RootPath)
		if err != nil {
			return ConflictDetail{}, err
		}
		for _, path := range paths {
			id, side, ok := ParseConflictFileName(filepath.Base(path))
			if !ok || id != itemID {
				continue
			}
			detail.Copies = append(detail.Copies, ConflictCopyView{
				Side:    side,
				Path:    path,
				Summary: readContentSummary(path),
			})
		}
	}
	return detail, nil
}
