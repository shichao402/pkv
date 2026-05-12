package app

import (
	"strings"
	"testing"

	bwtypes "github.com/shichao402/pkv/internal/bw/types"
)

func TestSplitBrowseResources(t *testing.T) {
	envField := bwtypes.CustomField{Name: bwtypes.PKVFieldName, Value: bwtypes.PKVTypeEnv}
	folder := bwtypes.Folder{ID: "folder-id", Name: "prod"}
	items := []bwtypes.Item{
		{ID: "ssh-1", Type: bwtypes.ItemTypeSSHKey, Name: "deploy"},
		{ID: "env-1", Type: bwtypes.ItemTypeSecureNote, Name: "pkv.env", Fields: []bwtypes.CustomField{envField}, Notes: "A=1"},
		{ID: "note-1", Type: bwtypes.ItemTypeSecureNote, Name: "app.conf", Notes: "debug=false"},
		{ID: "include-1", Type: bwtypes.ItemTypeSecureNote, Name: bwtypes.ReservedIncludeNoteName, Notes: "base"},
	}

	got, err := SplitBrowseResources(folder, items)
	if err != nil {
		t.Fatalf("SplitBrowseResources returned error: %v", err)
	}
	if got.Folder != folder {
		t.Fatalf("folder = %+v, want %+v", got.Folder, folder)
	}
	if len(got.SSHKeys) != 1 || got.SSHKeys[0].ID != "ssh-1" {
		t.Fatalf("SSHKeys = %+v, want ssh-1", got.SSHKeys)
	}
	if got.EnvNote == nil || got.EnvNote.ID != "env-1" {
		t.Fatalf("EnvNote = %+v, want env-1", got.EnvNote)
	}
	if len(got.Notes) != 1 || got.Notes[0].ID != "note-1" {
		t.Fatalf("Notes = %+v, want note-1 only", got.Notes)
	}
}

func TestSplitBrowseResourcesRejectsMultipleEnvNotes(t *testing.T) {
	items := []bwtypes.Item{
		{ID: "env-1", Type: bwtypes.ItemTypeSecureNote, Name: bwtypes.ReservedEnvNoteName},
		{ID: "env-2", Type: bwtypes.ItemTypeSecureNote, Name: bwtypes.ReservedEnvNoteName},
	}

	_, err := SplitBrowseResources(bwtypes.Folder{Name: "prod"}, items)
	if err == nil || !strings.Contains(err.Error(), "found 2 env notes") {
		t.Fatalf("SplitBrowseResources error = %v, want multiple-env error", err)
	}
}
