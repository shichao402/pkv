package guard

import (
	"testing"

	"github.com/shichao402/pkv/internal/bw/types"
	"github.com/shichao402/pkv/internal/state"
)

func TestDeriveConfigNeedsWorkspaceRootUnset(t *testing.T) {
	needs := DeriveConfigNeeds("", &state.State{}, nil)
	if len(needs) != 1 || needs[0].Code != "workspace_root_unset" {
		t.Fatalf("DeriveConfigNeeds() = %+v, want workspace_root_unset", needs)
	}
}

func TestDeriveConfigNeedsWorkspaceUnregistered(t *testing.T) {
	root := t.TempDir()
	needs := DeriveConfigNeeds(root, &state.State{}, nil)
	if len(needs) != 1 || needs[0].Code != "workspace_unregistered" {
		t.Fatalf("DeriveConfigNeeds() = %+v, want workspace_unregistered", needs)
	}
}

func TestDeriveConfigNeedsFolderNotFound(t *testing.T) {
	root := t.TempDir()
	st := &state.State{}
	if _, _, err := RegisterWorkspace(st, root, "missing-folder", ""); err != nil {
		t.Fatal(err)
	}
	folders := []types.Folder{{ID: "f1", Name: "other-folder"}}

	needs := DeriveConfigNeeds(root, st, folders)
	if len(needs) != 1 || needs[0].Code != "folder_not_found" {
		t.Fatalf("DeriveConfigNeeds() = %+v, want folder_not_found", needs)
	}
}

func TestDeriveConfigNeedsReady(t *testing.T) {
	root := t.TempDir()
	st := &state.State{}
	if _, _, err := RegisterWorkspace(st, root, "proj", ""); err != nil {
		t.Fatal(err)
	}
	folders := []types.Folder{{ID: "f1", Name: "proj"}}

	needs := DeriveConfigNeeds(root, st, folders)
	if len(needs) != 0 {
		t.Fatalf("DeriveConfigNeeds() = %+v, want none", needs)
	}
}

func TestStatusReadyAndNeedsConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PKV_WORKSPACE_ROOT", root)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BW_SESSION", "")

	st := &state.State{}
	g := New(st, nil, "")

	status := g.Status()
	if status.Ready {
		t.Fatal("Ready = true, want false before registration")
	}
	if len(status.NeedsConfig) != 1 || status.NeedsConfig[0].Code != "workspace_unregistered" {
		t.Fatalf("NeedsConfig = %+v, want workspace_unregistered", status.NeedsConfig)
	}

	if _, _, err := RegisterWorkspace(st, root, "proj", ""); err != nil {
		t.Fatal(err)
	}
	status = g.Status()
	if !status.Ready {
		t.Fatalf("Ready = false after registration, NeedsConfig = %+v", status.NeedsConfig)
	}
}
