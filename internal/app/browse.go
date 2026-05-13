package app

import (
	"context"
	"fmt"

	"github.com/shichao402/pkv/internal/bw"
	bwtypes "github.com/shichao402/pkv/internal/bw/types"
)

type BrowseResources struct {
	Folder  bwtypes.Folder
	Items   []bwtypes.Item
	SSHKeys []bwtypes.Item
	EnvNote *bwtypes.Item
	Notes   []bwtypes.Item
}

type BrowseSnapshot struct {
	Folders             []bwtypes.Folder
	ResourcesByFolderID map[string]BrowseResources
	ItemCount           int
}

type browseClient interface {
	EnsureUnlocked() (string, error)
	Sync(session string) error
	ListFolders(session string) ([]bwtypes.Folder, error)
	GetFolderID(session, name string) (string, error)
	ListItems(session, folderID string) ([]bwtypes.Item, error)
	ListAllItems(session string) ([]bwtypes.Item, error)
}

func BrowseFolders(ctx context.Context, r Reporter) ([]bwtypes.Folder, error) {
	return browseFoldersWithClient(ctx, bw.NewClient(), r)
}

func BrowseFolderResources(ctx context.Context, folder bwtypes.Folder, r Reporter) (BrowseResources, error) {
	return browseFolderResourcesWithClient(ctx, bw.NewClient(), folder, r)
}

func BrowseVaultSnapshot(ctx context.Context, r Reporter) (BrowseSnapshot, error) {
	return browseVaultSnapshotWithClient(ctx, bw.NewClient(), r)
}

func SplitBrowseResources(folder bwtypes.Folder, items []bwtypes.Item) (BrowseResources, error) {
	sshKeys := bw.FilterSSHKeys(items)
	envNote, hasEnv, err := bw.FindManagedEnvNote(items)
	if err != nil {
		return BrowseResources{}, err
	}
	notes := bw.FilterConfigNotes(items)

	result := BrowseResources{
		Folder:  folder,
		Items:   items,
		SSHKeys: sshKeys,
		Notes:   notes,
	}
	if hasEnv {
		result.EnvNote = &envNote
	}
	return result, nil
}

func browseFoldersWithClient(ctx context.Context, client browseClient, r Reporter) ([]bwtypes.Folder, error) {
	r = reporterOrNoop(r)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.Info("Authenticating with Bitwarden...")
	session, err := client.EnsureUnlocked()
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.Info("Syncing vault...")
	if err := client.Sync(session); err != nil {
		return nil, fmt.Errorf("sync failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.Info("Loading folders...")
	folders, err := client.ListFolders(session)
	if err != nil {
		return nil, fmt.Errorf("list folders failed: %w", err)
	}
	return folders, nil
}

func browseFolderResourcesWithClient(ctx context.Context, client browseClient, folder bwtypes.Folder, r Reporter) (BrowseResources, error) {
	r = reporterOrNoop(r)
	if err := ctx.Err(); err != nil {
		return BrowseResources{}, err
	}
	if folder.ID == "" && folder.Name == "" {
		return BrowseResources{}, fmt.Errorf("folder id or name is required")
	}

	r.Info("Authenticating with Bitwarden...")
	session, err := client.EnsureUnlocked()
	if err != nil {
		return BrowseResources{}, fmt.Errorf("authentication failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return BrowseResources{}, err
	}

	r.Info("Syncing vault...")
	if err := client.Sync(session); err != nil {
		return BrowseResources{}, fmt.Errorf("sync failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return BrowseResources{}, err
	}

	folderID := folder.ID
	if folderID == "" {
		r.Infof("Looking up folder '%s'...\n", folder.Name)
		folderID, err = client.GetFolderID(session, folder.Name)
		if err != nil {
			return BrowseResources{}, fmt.Errorf("folder lookup failed: %w", err)
		}
		folder.ID = folderID
	}

	r.Infof("Loading resources for '%s'...\n", folder.Name)
	items, err := client.ListItems(session, folderID)
	if err != nil {
		return BrowseResources{}, fmt.Errorf("list items failed: %w", err)
	}
	return SplitBrowseResources(folder, items)
}

func browseVaultSnapshotWithClient(ctx context.Context, client browseClient, r Reporter) (BrowseSnapshot, error) {
	r = reporterOrNoop(r)
	if err := ctx.Err(); err != nil {
		return BrowseSnapshot{}, err
	}

	r.Info("Authenticating with Bitwarden...")
	session, err := client.EnsureUnlocked()
	if err != nil {
		return BrowseSnapshot{}, fmt.Errorf("authentication failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return BrowseSnapshot{}, err
	}

	r.Info("Syncing vault...")
	if err := client.Sync(session); err != nil {
		return BrowseSnapshot{}, fmt.Errorf("sync failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return BrowseSnapshot{}, err
	}

	r.Info("Loading folders...")
	folders, err := client.ListFolders(session)
	if err != nil {
		return BrowseSnapshot{}, fmt.Errorf("list folders failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return BrowseSnapshot{}, err
	}

	r.Info("Loading all items...")
	items, err := client.ListAllItems(session)
	if err != nil {
		return BrowseSnapshot{}, fmt.Errorf("list items failed: %w", err)
	}

	itemsByFolderID := make(map[string][]bwtypes.Item)
	for _, item := range items {
		if item.FolderID == "" {
			continue
		}
		itemsByFolderID[item.FolderID] = append(itemsByFolderID[item.FolderID], item)
	}

	resourcesByFolderID := make(map[string]BrowseResources, len(folders))
	for _, folder := range folders {
		resources, err := SplitBrowseResources(folder, itemsByFolderID[folder.ID])
		if err != nil {
			return BrowseSnapshot{}, fmt.Errorf("folder %q: %w", folder.Name, err)
		}
		resourcesByFolderID[folder.ID] = resources
	}

	return BrowseSnapshot{
		Folders:             folders,
		ResourcesByFolderID: resourcesByFolderID,
		ItemCount:           len(items),
	}, nil
}
