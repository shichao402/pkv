package app

import (
	"context"
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

func TestEnrichBrowseResourcesResolvedThinProject(t *testing.T) {
	folders := []bwtypes.Folder{
		{ID: "root-id", Name: "root"},
		{ID: "shared-id", Name: "shared"},
	}
	itemsByFolderID := map[string][]bwtypes.Item{
		"root-id": {
			{ID: "inc-1", Type: bwtypes.ItemTypeSecureNote, Name: bwtypes.ReservedIncludeNoteName, Notes: "shared\n"},
		},
		"shared-id": {
			{ID: "env-1", Type: bwtypes.ItemTypeSecureNote, Name: bwtypes.ReservedEnvNoteName, Notes: "API_TOKEN=secret\n"},
			{ID: "note-1", Type: bwtypes.ItemTypeSecureNote, Name: "app.conf", Notes: "debug=true"},
		},
	}

	direct, err := SplitBrowseResources(folders[0], itemsByFolderID["root-id"])
	if err != nil {
		t.Fatalf("SplitBrowseResources returned error: %v", err)
	}
	got := enrichBrowseResourcesResolved(direct, folders, itemsByFolderID)

	if len(got.DirectIncludes) != 1 || got.DirectIncludes[0] != "shared" {
		t.Fatalf("DirectIncludes = %v, want [shared]", got.DirectIncludes)
	}
	if len(got.IncludeChain) != 2 || got.IncludeChain[0] != "root" || got.IncludeChain[1] != "shared" {
		t.Fatalf("IncludeChain = %v, want [root shared]", got.IncludeChain)
	}
	if got.ResolveErr != nil {
		t.Fatalf("ResolveErr = %v, want nil", got.ResolveErr)
	}
	if len(got.ResolvedEnv) != 1 || got.ResolvedEnv[0].Key != "API_TOKEN" || got.ResolvedEnv[0].Source != "shared" {
		t.Fatalf("ResolvedEnv = %+v, want API_TOKEN from shared", got.ResolvedEnv)
	}
	if len(got.ResolvedNotes) != 1 || got.ResolvedNotes[0].Item.Name != "app.conf" || got.ResolvedNotes[0].Source != "shared" {
		t.Fatalf("ResolvedNotes = %+v, want app.conf from shared", got.ResolvedNotes)
	}
}

func TestEnrichBrowseResourcesResolvedRecordsMissingInclude(t *testing.T) {
	folders := []bwtypes.Folder{{ID: "root-id", Name: "root"}}
	itemsByFolderID := map[string][]bwtypes.Item{
		"root-id": {
			{ID: "inc-1", Type: bwtypes.ItemTypeSecureNote, Name: bwtypes.ReservedIncludeNoteName, Notes: "missing\n"},
		},
	}
	direct, err := SplitBrowseResources(folders[0], itemsByFolderID["root-id"])
	if err != nil {
		t.Fatalf("SplitBrowseResources returned error: %v", err)
	}
	got := enrichBrowseResourcesResolved(direct, folders, itemsByFolderID)
	if got.ResolveErr == nil {
		t.Fatal("ResolveErr = nil, want missing-folder error")
	}
	if !strings.Contains(got.ResolveErr.Error(), "missing folders") {
		t.Fatalf("ResolveErr = %v, want missing folders", got.ResolveErr)
	}
}

func TestBrowseVaultSnapshotResolvesIncludeAcrossFolders(t *testing.T) {
	client := &fakeBrowseClient{
		folders: []bwtypes.Folder{
			{ID: "root-id", Name: "root"},
			{ID: "shared-id", Name: "shared"},
		},
		allItems: []bwtypes.Item{
			{ID: "inc-1", FolderID: "root-id", Type: bwtypes.ItemTypeSecureNote, Name: bwtypes.ReservedIncludeNoteName, Notes: "shared\n"},
			{ID: "env-1", FolderID: "shared-id", Type: bwtypes.ItemTypeSecureNote, Name: bwtypes.ReservedEnvNoteName, Notes: "FOO=bar\n"},
			{ID: "note-1", FolderID: "shared-id", Type: bwtypes.ItemTypeSecureNote, Name: "cfg.yml", Notes: "x: 1"},
		},
	}

	got, err := browseVaultSnapshotWithClient(context.Background(), client, nil)
	if err != nil {
		t.Fatalf("browseVaultSnapshotWithClient returned error: %v", err)
	}
	root := got.ResourcesByFolderID["root-id"]
	if len(root.ResolvedEnv) != 1 {
		t.Fatalf("root ResolvedEnv = %+v, want 1 key", root.ResolvedEnv)
	}
	if root.ResolvedEnv[0].Key != "FOO" {
		t.Fatalf("ResolvedEnv key = %q, want FOO", root.ResolvedEnv[0].Key)
	}
	if len(root.ResolvedNotes) != 1 || root.ResolvedNotes[0].Item.Name != "cfg.yml" {
		t.Fatalf("root ResolvedNotes = %+v, want cfg.yml", root.ResolvedNotes)
	}
}

func TestBrowseVaultSnapshotGroupsItemsByFolderID(t *testing.T) {
	client := &fakeBrowseClient{
		folders: []bwtypes.Folder{
			{ID: "f-ssh", Name: "ssh"},
			{ID: "f-prod", Name: "prod"},
		},
		allItems: []bwtypes.Item{
			{ID: "ssh-1", FolderID: "f-ssh", Type: bwtypes.ItemTypeSSHKey, Name: "deploy"},
			{ID: "env-1", FolderID: "f-prod", Type: bwtypes.ItemTypeSecureNote, Name: bwtypes.ReservedEnvNoteName, Notes: "A=1"},
			{ID: "unfiled-1", Type: bwtypes.ItemTypeSecureNote, Name: "ignored"},
		},
	}

	got, err := browseVaultSnapshotWithClient(context.Background(), client, nil)
	if err != nil {
		t.Fatalf("browseVaultSnapshotWithClient returned error: %v", err)
	}
	if got.ItemCount != 3 {
		t.Fatalf("ItemCount = %d, want 3", got.ItemCount)
	}
	if client.listAllItemsCalls != 1 {
		t.Fatalf("ListAllItems calls = %d, want 1", client.listAllItemsCalls)
	}
	if client.listItemsCalls != 0 {
		t.Fatalf("ListItems calls = %d, want 0", client.listItemsCalls)
	}
	ssh := got.ResourcesByFolderID["f-ssh"]
	if len(ssh.SSHKeys) != 1 || ssh.SSHKeys[0].ID != "ssh-1" {
		t.Fatalf("ssh resources = %+v, want ssh-1", ssh)
	}
	prod := got.ResourcesByFolderID["f-prod"]
	if prod.EnvNote == nil || prod.EnvNote.ID != "env-1" {
		t.Fatalf("prod env = %+v, want env-1", prod.EnvNote)
	}
}

type fakeBrowseClient struct {
	folders  []bwtypes.Folder
	allItems []bwtypes.Item

	listAllItemsCalls int
	listItemsCalls    int
}

func (c *fakeBrowseClient) EnsureUnlocked() (string, error) {
	return "session", nil
}

func (c *fakeBrowseClient) Sync(string) error {
	return nil
}

func (c *fakeBrowseClient) ListFolders(string) ([]bwtypes.Folder, error) {
	return c.folders, nil
}

func (c *fakeBrowseClient) GetFolderID(string, string) (string, error) {
	return "", nil
}

func (c *fakeBrowseClient) ListItems(string, string) ([]bwtypes.Item, error) {
	c.listItemsCalls++
	return nil, nil
}

func (c *fakeBrowseClient) ListAllItems(string) ([]bwtypes.Item, error) {
	c.listAllItemsCalls++
	return c.allItems, nil
}
