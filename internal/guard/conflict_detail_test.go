package guard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shichao402/pkv/internal/state"
)

func TestShowConflict(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "notes")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(target, "demo.md")
	if err := os.WriteFile(canonical, []byte("canonical body"), 0o600); err != nil {
		t.Fatal(err)
	}
	localCopy, err := WriteConflictCopy(root, "item-1", "local", "demo.md", []byte("local copy body"))
	if err != nil {
		t.Fatal(err)
	}

	st := &state.State{}
	st.RegisterWorkspace(state.WorkspaceEntry{
		RootPath:  root,
		Folder:    "proj",
		TargetDir: target,
	})
	st.UpsertNote(state.NoteEntry{
		ItemID:    "item-1",
		Folder:    "proj",
		TargetDir: target,
		FileName:  "demo.md",
		FilePath:  canonical,
		Conflict:  state.ConflictPending,
	})

	detail, err := ShowConflict(st, "item-1")
	if err != nil {
		t.Fatalf("ShowConflict: %v", err)
	}
	if detail.CanonicalPath != canonical {
		t.Fatalf("canonical path = %q, want %q", detail.CanonicalPath, canonical)
	}
	if detail.CanonicalSummary != "canonical body" {
		t.Fatalf("canonical summary = %q", detail.CanonicalSummary)
	}
	if len(detail.Copies) != 1 || detail.Copies[0].Path != localCopy {
		t.Fatalf("copies = %#v, want path %q", detail.Copies, localCopy)
	}
}

func TestSummarizeContent(t *testing.T) {
	long := strings.Repeat("x", 300)
	if got := SummarizeContent(long); len([]rune(got)) > contentSummaryMaxLen+3 {
		t.Fatalf("summary too long: %d", len(got))
	}
}
