package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shichao402/pkv/internal/app"
	bwtypes "github.com/shichao402/pkv/internal/bw/types"
)

func TestUpdateFoldersLoadedSelectsFirstFolder(t *testing.T) {
	model := NewModel(t.Context())
	updated, cmd := model.Update(foldersLoadedMsg{folders: []bwtypes.Folder{{ID: "folder-1", Name: "prod"}}})
	got := updated.(Model)

	if got.loading != true {
		t.Fatal("loading = false, want true while folder resources load")
	}
	if got.currentFolder == nil || got.currentFolder.Name != "prod" {
		t.Fatalf("currentFolder = %+v, want prod", got.currentFolder)
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want resource load command")
	}
}

func TestUpdateKeySwitchesResourceTabs(t *testing.T) {
	model := NewModel(t.Context())
	model.focus = focusResources
	model.tab = tabSSH

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	got := updated.(Model)
	if got.tab != tabEnv {
		t.Fatalf("tab = %v, want env", got.tab)
	}

	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyRight})
	got = updated.(Model)
	if got.tab != tabNotes {
		t.Fatalf("tab = %v, want notes", got.tab)
	}
}

func TestViewRendersFolderAndResourceTabs(t *testing.T) {
	model := NewModel(t.Context())
	folder := bwtypes.Folder{ID: "folder-1", Name: "prod"}
	model.loading = false
	model.folders = []bwtypes.Folder{folder}
	model.currentFolder = &folder
	model.resources = app.BrowseResources{
		Folder:  folder,
		SSHKeys: []bwtypes.Item{{ID: "ssh-123456789", Type: bwtypes.ItemTypeSSHKey, Name: "deploy"}},
		Notes:   []bwtypes.Item{{ID: "note-1", Type: bwtypes.ItemTypeSecureNote, Name: "app.conf"}},
	}

	view := model.View()
	for _, want := range []string{"PKV", "prod", "SSH", "Env", "Notes", "deploy"} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q in %q", want, view)
		}
	}
}

func TestEnvKeysOmitsValues(t *testing.T) {
	got := envKeys("API_TOKEN=secret\n# comment\nEMPTY=\nBAD")
	want := []string{"API_TOKEN", "EMPTY"}
	if len(got) != len(want) {
		t.Fatalf("envKeys length = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("envKeys[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
