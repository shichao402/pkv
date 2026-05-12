package tui

import (
	"context"
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

func TestDeleteStartsConfirmationAndCancelStopsIt(t *testing.T) {
	model := readyModel()
	model.focus = focusDetail
	model.tab = tabSSH

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("delete key cmd = non-nil, want confirmation only")
	}
	if got.interaction != interactionConfirm || got.confirm.kind != confirmRemove {
		t.Fatalf("interaction = %v/%v, want remove confirmation", got.interaction, got.confirm.kind)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("cancel cmd = non-nil, want no destructive command")
	}
	if got.interaction != interactionNone {
		t.Fatalf("interaction = %v, want none", got.interaction)
	}
}

func TestDeleteEnvWithoutItemDoesNotConfirm(t *testing.T) {
	model := readyModel()
	model.tab = tabEnv
	model.resources.EnvNote = nil

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("delete env cmd = non-nil, want no command")
	}
	if got.interaction != interactionNone {
		t.Fatalf("interaction = %v, want none", got.interaction)
	}
}

func TestConfirmRemoveReturnsOperationCommand(t *testing.T) {
	model := readyModel()
	model.focus = focusDetail
	model.tab = tabSSH
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = updated.(Model)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	got := updated.(Model)
	if cmd == nil {
		t.Fatal("confirm cmd = nil, want operation command")
	}
	if !got.loading {
		t.Fatal("loading = false, want true while remove runs")
	}
}

func TestEditEnvCanStartWithoutExistingEnv(t *testing.T) {
	model := readyModel()
	model.resources.EnvNote = nil
	model.tab = tabEnv
	model.focus = focusResources

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("edit env cmd = non-nil, want edit state only")
	}
	if got.interaction != interactionEdit {
		t.Fatalf("interaction = %v, want edit", got.interaction)
	}
	if got.edit.item.Name != bwtypes.ReservedEnvNoteName {
		t.Fatalf("edit item = %q, want reserved env note", got.edit.item.Name)
	}
}

func TestSSHWizardStartsAndEscCancels(t *testing.T) {
	model := readyModel()
	model.tab = tabSSH
	model.focus = focusResources

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	got := updated.(Model)
	if got.interaction != interactionSSHWizard || got.sshWizard.step != sshStepPrivatePath {
		t.Fatalf("interaction = %v step = %v, want ssh wizard private path", got.interaction, got.sshWizard.step)
	}

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("escape cmd = non-nil, want no command")
	}
	if got.interaction != interactionNone {
		t.Fatalf("interaction = %v, want none", got.interaction)
	}
}

func TestOperationResultReloadsCurrentFolder(t *testing.T) {
	model := readyModel()
	updated, cmd := model.Update(operationResultMsg{message: "saved", reload: true})
	got := updated.(Model)
	if !got.loading {
		t.Fatal("loading = false, want true while resources reload")
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want resource reload command")
	}
}

func readyModel() Model {
	model := NewModel(context.TODO())
	folder := bwtypes.Folder{ID: "folder-1", Name: "prod"}
	model.loading = false
	model.folders = []bwtypes.Folder{folder}
	model.currentFolder = &folder
	model.resources = app.BrowseResources{
		Folder:  folder,
		SSHKeys: []bwtypes.Item{{ID: "ssh-123456789", Type: bwtypes.ItemTypeSSHKey, Name: "deploy"}},
		EnvNote: &bwtypes.Item{ID: "env-1", Type: bwtypes.ItemTypeSecureNote, Name: bwtypes.ReservedEnvNoteName, Notes: "API_TOKEN=secret"},
		Notes:   []bwtypes.Item{{ID: "note-1", Type: bwtypes.ItemTypeSecureNote, Name: "app.conf", Notes: "debug=true"}},
	}
	return model
}
