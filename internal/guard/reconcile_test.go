package guard

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shichao402/pkv/internal/bw/types"
	"github.com/shichao402/pkv/internal/state"
)

func TestReconcileNotePullRemote(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	entry := state.NoteEntry{ItemID: "id-1", FileName: "note.md", FilePath: path}
	remote := types.Item{
		ID:           "id-1",
		Notes:        "remote body",
		RevisionDate: time.Now().UTC().Format(time.RFC3339),
	}

	out, err := ReconcileNote(ReconcileInput{
		Entry:  entry,
		Remote: remote,
	}, ReconcileDecision{Action: ActionPullRemote}, nil)
	if err != nil {
		t.Fatalf("ReconcileNote() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "remote body" {
		t.Fatalf("file content = %q, want remote body", string(data))
	}
	if out.UpdatedEntry.ContentHash != hashContent("remote body") {
		t.Fatal("content hash not updated")
	}
}
