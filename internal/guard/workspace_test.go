package guard

import (
	"testing"

	"github.com/shichao402/pkv/internal/state"
)

func TestRegisterWorkspaceDuplicateReturnsExisting(t *testing.T) {
	st := &state.State{}
	root := t.TempDir()

	first, already, err := RegisterWorkspace(st, root, "proj-a", "")
	if err != nil {
		t.Fatalf("first RegisterWorkspace() error = %v", err)
	}
	if already {
		t.Fatal("already = true on first registration")
	}

	second, already, err := RegisterWorkspace(st, root, "proj-a", "")
	if err != nil {
		t.Fatalf("second RegisterWorkspace() error = %v", err)
	}
	if !already {
		t.Fatal("already = false on duplicate registration")
	}
	if second.RootPath != first.RootPath || second.Folder != first.Folder {
		t.Fatalf("duplicate entry = %+v, want %+v", second, first)
	}
	if len(st.ListWorkspaces()) != 1 {
		t.Fatalf("workspace count = %d, want 1", len(st.ListWorkspaces()))
	}
}

func TestRegisterWorkspaceSamePathDifferentFolderErrors(t *testing.T) {
	st := &state.State{}
	root := t.TempDir()

	if _, _, err := RegisterWorkspace(st, root, "proj-a", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := RegisterWorkspace(st, root, "proj-b", ""); err == nil {
		t.Fatal("expected error for same path with different folder")
	}
}
