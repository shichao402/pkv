package bw

import (
	"strings"
	"testing"

	"github.com/shichao402/pkv/internal/bw/types"
)

func TestLoadIncludeChainFromVault_SimpleChain(t *testing.T) {
	folders := []types.Folder{
		{ID: "root-id", Name: "root"},
		{ID: "a-id", Name: "a"},
		{ID: "b-id", Name: "b"},
	}
	itemsByFolderID := map[string][]types.Item{
		"root-id": {
			{ID: "inc", Type: types.ItemTypeSecureNote, Name: types.ReservedIncludeNoteName, Notes: "a\nb\n"},
		},
	}

	got, err := LoadIncludeChainFromVault("root", folders, itemsByFolderID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("chain len = %d, want 3", len(got))
	}
	names := []string{got[0].Name, got[1].Name, got[2].Name}
	want := []string{"root", "a", "b"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("chain[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}

func TestLoadIncludeChainFromVault_MissingInclude(t *testing.T) {
	folders := []types.Folder{{ID: "root-id", Name: "root"}}
	itemsByFolderID := map[string][]types.Item{
		"root-id": {
			{ID: "inc", Type: types.ItemTypeSecureNote, Name: types.ReservedIncludeNoteName, Notes: "ghost\n"},
		},
	}
	_, err := LoadIncludeChainFromVault("root", folders, itemsByFolderID)
	if err == nil {
		t.Fatal("expected missing-folder error")
	}
	if !strings.Contains(err.Error(), "missing folders") {
		t.Fatalf("error = %v, want missing folders", err)
	}
}
