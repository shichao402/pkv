package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shichao402/pkv/internal/app"
	bwtypes "github.com/shichao402/pkv/internal/bw/types"
)

func TestUpdateVaultLoadedSelectsFirstFolder(t *testing.T) {
	model := NewModel(t.Context())
	snapshot := app.BrowseSnapshot{
		Folders: []bwtypes.Folder{{ID: "folder-1", Name: "prod"}},
		ResourcesByFolderID: map[string]app.BrowseResources{
			"folder-1": {
				Folder:  bwtypes.Folder{ID: "folder-1", Name: "prod"},
				SSHKeys: []bwtypes.Item{{ID: "ssh-1", Type: bwtypes.ItemTypeSSHKey, Name: "deploy"}},
			},
		},
		ItemCount: 1,
	}

	updated, cmd := model.Update(vaultLoadedMsg{requestID: model.activeLoadID, snapshot: snapshot})
	got := updated.(Model)

	if got.loading {
		t.Fatal("loading = true, want false after vault snapshot loads")
	}
	if got.currentFolder == nil || got.currentFolder.Name != "prod" {
		t.Fatalf("currentFolder = %+v, want prod", got.currentFolder)
	}
	if len(got.resources.SSHKeys) != 1 || got.resources.SSHKeys[0].ID != "ssh-1" {
		t.Fatalf("SSHKeys = %+v, want ssh-1", got.resources.SSHKeys)
	}
	if cmd != nil {
		t.Fatal("cmd != nil, want no follow-up resource load")
	}
}

func TestFolderSelectionUsesCachedResources(t *testing.T) {
	model := readyModel()
	model.focus = focusFolders
	model.folders = []bwtypes.Folder{
		{ID: "folder-1", Name: "prod"},
		{ID: "folder-2", Name: "ssh"},
	}
	model.resourcesByFolderID = map[string]app.BrowseResources{
		"folder-1": model.resources,
		"folder-2": {
			Folder:  bwtypes.Folder{ID: "folder-2", Name: "ssh"},
			SSHKeys: []bwtypes.Item{{ID: "ssh-2", Type: bwtypes.ItemTypeSSHKey, Name: "admin"}},
		},
	}

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	got := updated.(Model)

	if cmd != nil {
		t.Fatal("down key cmd != nil, want cached local selection only")
	}
	if got.currentFolder == nil || got.currentFolder.Name != "ssh" {
		t.Fatalf("currentFolder = %+v, want ssh", got.currentFolder)
	}
	if len(got.resources.SSHKeys) != 1 || got.resources.SSHKeys[0].ID != "ssh-2" {
		t.Fatalf("SSHKeys = %+v, want ssh-2", got.resources.SSHKeys)
	}
}

func TestStaleVaultLoadedMessageIsIgnored(t *testing.T) {
	model := readyModel()
	model.activeLoadID = 2
	model.loadSeq = 2
	oldFolder := model.currentFolder.Name

	updated, cmd := model.Update(vaultLoadedMsg{
		requestID: 1,
		snapshot: app.BrowseSnapshot{
			Folders: []bwtypes.Folder{{ID: "old", Name: "old"}},
		},
	})
	got := updated.(Model)

	if cmd != nil {
		t.Fatal("cmd != nil, want stale load ignored")
	}
	if got.currentFolder == nil || got.currentFolder.Name != oldFolder {
		t.Fatalf("currentFolder = %+v, want unchanged %q", got.currentFolder, oldFolder)
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

func TestGetStartsConfirmationAndCancelStopsIt(t *testing.T) {
	model := readyModel()
	model.focus = focusResources
	model.tab = tabEnv

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("get key cmd = non-nil, want confirmation only")
	}
	if got.interaction != interactionConfirm || got.confirm.kind != confirmGet {
		t.Fatalf("interaction = %v/%v, want get confirmation", got.interaction, got.confirm.kind)
	}
	if got.confirm.tab != tabEnv {
		t.Fatalf("confirm tab = %v, want env", got.confirm.tab)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("cancel cmd = non-nil, want no get command")
	}
	if got.interaction != interactionNone {
		t.Fatalf("interaction = %v, want none", got.interaction)
	}
}

func TestConfirmGetReturnsOperationCommand(t *testing.T) {
	model := readyModel()
	model.focus = focusResources
	model.tab = tabNotes
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	model = updated.(Model)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	got := updated.(Model)
	if cmd == nil {
		t.Fatal("confirm get cmd = nil, want operation command")
	}
	if !got.loading {
		t.Fatal("loading = false, want true while get runs")
	}
	if got.status != "Getting..." {
		t.Fatalf("status = %q, want Getting...", got.status)
	}
}

func TestViewRendersGetHint(t *testing.T) {
	model := readyModel()
	model.focus = focusResources

	view := model.View()
	if !strings.Contains(view, "g get") {
		t.Fatalf("View() missing get hint in %q", view)
	}
}

func TestHelpPopupOpensAndCloses(t *testing.T) {
	model := readyModel()

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("help key cmd = non-nil, want local state update only")
	}
	if !got.helpOpen {
		t.Fatal("helpOpen = false, want true")
	}
	if view := got.View(); !strings.Contains(view, "Keyboard Help") || !strings.Contains(view, "?/esc close") {
		t.Fatalf("View() missing help popup in %q", view)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("escape help cmd = non-nil, want local state update only")
	}
	if got.helpOpen {
		t.Fatal("helpOpen = true, want false")
	}
}

func TestEditEnvCanStartWithoutExistingEnv(t *testing.T) {
	model := readyModel()
	model.resources.EnvNote = nil
	model.tab = tabEnv
	model.focus = focusResources

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	got := updated.(Model)
	if cmd == nil {
		t.Fatal("edit env cmd = nil, want external editor command")
	}
	if got.interaction != interactionNone {
		t.Fatalf("interaction = %v, want none while external editor opens", got.interaction)
	}
	if got.edit.item.Name != bwtypes.ReservedEnvNoteName {
		t.Fatalf("edit item = %q, want reserved env note", got.edit.item.Name)
	}
}

func TestEditorFinishedWithChangesReturnsSaveCommand(t *testing.T) {
	model := readyModel()
	state := editState{tab: tabEnv, item: *model.resources.EnvNote, content: newTextBuffer(model.resources.EnvNote.Notes)}

	updated, cmd := model.Update(editorFinishedMsg{state: state, content: "API_TOKEN=changed"})
	got := updated.(Model)
	if cmd == nil {
		t.Fatal("editor finished cmd = nil, want save command")
	}
	if !got.loading {
		t.Fatal("loading = false, want true while save runs")
	}
	if got.status != "Saving..." {
		t.Fatalf("status = %q, want Saving...", got.status)
	}
}

func TestAddFolderStartsFromFolderListAndEscCancels(t *testing.T) {
	model := readyModel()
	model.focus = focusFolders

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("add folder cmd = non-nil, want input mode only")
	}
	if got.interaction != interactionAddFolder {
		t.Fatalf("interaction = %v, want add folder", got.interaction)
	}

	updated, cmd = got.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got = updated.(Model)
	if cmd != nil {
		t.Fatal("escape cmd = non-nil, want no command")
	}
	if got.interaction != interactionNone {
		t.Fatalf("interaction = %v, want none", got.interaction)
	}
}

func TestAddFolderRejectsEmptyInputInTUI(t *testing.T) {
	model := readyModel()
	model.focus = focusFolders
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = updated.(Model)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("empty folder enter cmd = non-nil, want validation only")
	}
	if got.interaction != interactionAddFolder {
		t.Fatalf("interaction = %v, want add folder", got.interaction)
	}
	if !strings.Contains(got.addFolder.err, "cannot be empty") {
		t.Fatalf("addFolder.err = %q, want empty validation", got.addFolder.err)
	}
}

func TestAddFolderWithNameReturnsOperationCommand(t *testing.T) {
	model := readyModel()
	model.focus = focusFolders
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("dev")})
	model = updated.(Model)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if cmd == nil {
		t.Fatal("folder create cmd = nil, want operation command")
	}
	if !got.loading {
		t.Fatal("loading = false, want true while folder create runs")
	}
	if got.interaction != interactionNone {
		t.Fatalf("interaction = %v, want none", got.interaction)
	}
}

func TestSSHWizardBlankPrivatePathGeneratesKey(t *testing.T) {
	model := readyModel()
	model.tab = tabSSH
	model.focus = focusResources

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("enter cmd = non-nil, want local wizard transition")
	}
	if got.sshWizard.step != sshStepPublicKey {
		t.Fatalf("step = %v, want public key", got.sshWizard.step)
	}
	if !got.sshWizard.generated {
		t.Fatal("generated = false, want true")
	}
	if !strings.Contains(got.sshWizard.openSSHKey, "OPENSSH PRIVATE KEY") {
		t.Fatal("openSSHKey missing OpenSSH private key")
	}
	if !strings.HasPrefix(got.sshWizard.derivedPub, "ssh-ed25519 ") {
		t.Fatalf("derivedPub = %q, want ssh-ed25519 key", got.sshWizard.derivedPub)
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

func TestOperationResultReloadsVault(t *testing.T) {
	model := readyModel()
	oldLoadID := model.activeLoadID

	updated, cmd := model.Update(operationResultMsg{message: "saved", reload: true})
	got := updated.(Model)
	if !got.loading {
		t.Fatal("loading = false, want true while vault reloads")
	}
	if got.activeLoadID == oldLoadID {
		t.Fatalf("activeLoadID = %d, want a new load id", got.activeLoadID)
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want vault reload command")
	}
}

func TestReloadKeyReloadsVault(t *testing.T) {
	model := readyModel()
	oldLoadID := model.activeLoadID

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	got := updated.(Model)
	if !got.loading {
		t.Fatal("loading = false, want true while vault reloads")
	}
	if got.activeLoadID == oldLoadID {
		t.Fatalf("activeLoadID = %d, want a new load id", got.activeLoadID)
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want vault reload command")
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
	model.resourcesByFolderID = map[string]app.BrowseResources{folder.ID: model.resources}
	return model
}
