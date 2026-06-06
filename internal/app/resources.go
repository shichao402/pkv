package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shichao402/pkv/internal/bw"
	bwtypes "github.com/shichao402/pkv/internal/bw/types"
	"github.com/shichao402/pkv/internal/env"
	"github.com/shichao402/pkv/internal/include"
	"github.com/shichao402/pkv/internal/key"
	"github.com/shichao402/pkv/internal/note"
	"github.com/shichao402/pkv/internal/securenote"
	"github.com/shichao402/pkv/internal/ssh"
	"github.com/shichao402/pkv/internal/state"
)

type ListParams struct {
	Folder   string
	Resolved bool
}

type ListResult struct {
	FolderCount int
	ItemCount   int
}

func List(ctx context.Context, params ListParams, r Reporter) (ListResult, error) {
	r = reporterOrNoop(r)
	if err := ctx.Err(); err != nil {
		return ListResult{}, err
	}
	if params.Folder == "" {
		return listFolders(ctx, r)
	}
	return listFolder(ctx, params, r)
}

func listFolders(ctx context.Context, r Reporter) (ListResult, error) {
	client := bw.NewClient()

	session, err := ensureBitwardenSession(ctx, client, r)
	if err != nil {
		return ListResult{}, fmt.Errorf("authentication failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return ListResult{}, err
	}

	r.Info("Syncing vault...")
	if err := client.Sync(session); err != nil {
		return ListResult{}, fmt.Errorf("sync failed: %w", err)
	}

	folders, err := client.ListFolders(session)
	if err != nil {
		return ListResult{}, fmt.Errorf("list folders failed: %w", err)
	}
	if len(folders) == 0 {
		r.Info("No folders found.")
		return ListResult{}, nil
	}

	r.Info("")
	r.Info("Folders:")
	r.Info("")
	r.Infof("%-36s  %s\n", "ID", "Name")
	r.Infof("%-36s  %s\n", "----", "----")
	for _, folder := range folders {
		r.Infof("%-36s  %s\n", folder.ID, folder.Name)
	}
	r.Infof("\n%d folder(s) found.\n", len(folders))
	return ListResult{FolderCount: len(folders)}, nil
}

func listFolder(ctx context.Context, params ListParams, r Reporter) (ListResult, error) {
	folder := params.Folder
	client := bw.NewClient()

	session, err := ensureBitwardenSession(ctx, client, r)
	if err != nil {
		return ListResult{}, fmt.Errorf("authentication failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return ListResult{}, err
	}

	r.Info("Syncing vault...")
	if err := client.Sync(session); err != nil {
		return ListResult{}, fmt.Errorf("sync failed: %w", err)
	}

	r.Infof("Looking up folder '%s'...\n", folder)
	folderID, err := client.GetFolderID(session, folder)
	if err != nil {
		return ListResult{}, fmt.Errorf("folder lookup failed: %w", err)
	}

	items, err := client.ListItems(session, folderID)
	if err != nil {
		return ListResult{}, fmt.Errorf("list items failed: %w", err)
	}

	sshKeys := bw.FilterSSHKeys(items)
	envItem, hasEnv, err := bw.FindManagedEnvNote(items)
	if err != nil {
		return ListResult{}, err
	}
	notes := bw.FilterConfigNotes(items)

	includeItem, hasInclude, err := include.FindIncludeNote(items)
	if err != nil {
		return ListResult{}, err
	}
	var directIncludes []string
	if hasInclude {
		directIncludes = include.ParseLines(includeItem.Notes)
	}

	r.Infof("\nFolder '%s'\n\n", folder)
	r.Infof("SSH keys: %d\n", len(sshKeys))
	if hasEnv {
		r.Infof("Env note: %s (%s)\n", envItem.Name, envItem.ID)
	} else {
		r.Infof("Env note: none (create one named '%s')\n", bwtypes.ReservedEnvNoteName)
	}
	r.Infof("Config notes: %d\n", len(notes))

	if len(directIncludes) > 0 {
		r.Info("\nIncludes:")
		for _, name := range directIncludes {
			r.Infof("  %s\n", name)
		}
	}
	if len(sshKeys) > 0 {
		r.Info("\nSSH:")
		for _, item := range sshKeys {
			r.Infof("  %s  %s\n", item.ID, item.Name)
		}
	}
	if len(notes) > 0 {
		r.Info("\nNotes:")
		for _, item := range notes {
			r.Infof("  %s  %s\n", item.ID, item.Name)
		}
	}

	if !params.Resolved {
		return ListResult{ItemCount: len(items)}, nil
	}

	chain, err := client.LoadIncludeChain(session, folder)
	if err != nil {
		return ListResult{}, fmt.Errorf("resolve include chain: %w", err)
	}
	chainNames := make([]string, len(chain))
	for i, f := range chain {
		chainNames[i] = f.Name
	}

	notesByFolder := make(map[string]string, len(chain))
	itemsByFolder := make(map[string][]bwtypes.Item, len(chain))
	for _, f := range chain {
		var chainItems []bwtypes.Item
		if f.ID == folderID {
			chainItems = items
		} else {
			chainItems, err = client.ListItems(session, f.ID)
			if err != nil {
				return ListResult{}, fmt.Errorf("list items for folder '%s': %w", f.Name, err)
			}
		}
		itemsByFolder[f.Name] = bw.FilterConfigNotes(chainItems)
		envNote, found, err := bw.FindManagedEnvNote(chainItems)
		if err != nil {
			return ListResult{}, fmt.Errorf("folder '%s': %w", f.Name, err)
		}
		if found {
			notesByFolder[f.Name] = envNote.Notes
		}
	}

	r.Info("\nResolved view (env + note across pkv.include chain):")
	if len(chain) > 1 {
		r.Infof("  已展开 include：%s\n", strings.Join(chainNames, " -> "))
	} else {
		r.Info("  (no pkv.include on this folder; showing current folder only)")
	}

	if len(notesByFolder) == 0 {
		r.Info("\n  Env: none")
	} else {
		envResult, err := env.MergePkvEnvNotes(chainNames, notesByFolder)
		if err != nil {
			return ListResult{}, err
		}
		r.Infof("\n  Env (%d key(s)):\n", len(envResult.Vars))
		for _, v := range envResult.Vars {
			r.Infof("    [from: %s] %s\n", v.Source, v.Key)
		}
		if len(envResult.Conflicts) > 0 {
			r.Info("\n  Env overrides:")
			for _, c := range envResult.Conflicts {
				r.Infof("    %s: winner=%s shadowed=%s\n", c.Key, c.Winner, strings.Join(c.Losers, ","))
			}
		}
	}

	merged := note.MergeNoteItems(chainNames, itemsByFolder)
	r.Infof("\n  Notes (%d item(s)):\n", len(merged.Items))
	for _, it := range merged.Items {
		r.Infof("    [from: %s] %s\n", it.Source, it.Item.Name)
	}
	if len(merged.Conflicts) > 0 {
		r.Info("\n  Note overrides:")
		for _, c := range merged.Conflicts {
			r.Infof("    %s: winner=%s shadowed=%s\n", c.Name, c.Winner, strings.Join(c.Losers, ","))
		}
	}
	r.Infof("\n  SSH: %d key(s) (not expanded through pkv.include; MVP)\n", len(sshKeys))
	return ListResult{ItemCount: len(items)}, nil
}

type GetParams struct {
	Folder       string
	FolderID     string
	Kind         string
	AuthorizeSSH bool
}

type GetResult struct {
	SSHDeployed int
	Authorized  int
	EnvKeys     int
	NotesSynced int
}

func Get(ctx context.Context, params GetParams, r Reporter) (GetResult, error) {
	r = reporterOrNoop(r)
	switch params.Kind {
	case "ssh":
		return GetSSH(ctx, params, r)
	case "env":
		return GetEnv(ctx, params, r)
	case "note":
		return GetNote(ctx, params, r)
	case "all":
		return GetAll(ctx, params, r)
	default:
		return GetResult{}, fmt.Errorf("unknown resource type: %s (expected ssh, env, note, or all)", params.Kind)
	}
}

func GetAll(ctx context.Context, params GetParams, r Reporter) (GetResult, error) {
	r = reporterOrNoop(r)
	var errs []error
	var result GetResult

	r.Info("=== SSH Keys ===")
	sshResult, err := GetSSH(ctx, params, r)
	if err != nil {
		r.Errorf("get ssh failed: %v\n", err)
		errs = append(errs, fmt.Errorf("ssh: %w", err))
	} else {
		result.SSHDeployed = sshResult.SSHDeployed
		result.Authorized = sshResult.Authorized
	}

	r.Info("\n=== Env Artifacts ===")
	envResult, err := GetEnv(ctx, params, r)
	if err != nil {
		r.Errorf("get env failed: %v\n", err)
		errs = append(errs, fmt.Errorf("env: %w", err))
	} else {
		result.EnvKeys = envResult.EnvKeys
	}

	r.Info("\n=== Config Notes ===")
	noteResult, err := GetNote(ctx, params, r)
	if err != nil {
		r.Errorf("get note failed: %v\n", err)
		errs = append(errs, fmt.Errorf("note: %w", err))
	} else {
		result.NotesSynced = noteResult.NotesSynced
	}

	return result, errors.Join(errs...)
}

func GetSSH(ctx context.Context, params GetParams, r Reporter) (GetResult, error) {
	folder := params.Folder
	client := bw.NewClient()

	session, err := ensureBitwardenSession(ctx, client, r)
	if err != nil {
		return GetResult{}, fmt.Errorf("authentication failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return GetResult{}, err
	}

	r.Info("Syncing vault...")
	if err := client.Sync(session); err != nil {
		return GetResult{}, fmt.Errorf("sync failed: %w", err)
	}

	r.Infof("Looking up folder '%s'...\n", folder)
	folderID, err := resolveFolderID(client, session, folder, params.FolderID)
	if err != nil {
		return GetResult{}, fmt.Errorf("folder lookup failed: %w", err)
	}

	r.Info("Listing SSH keys...")
	items, err := client.ListItems(session, folderID)
	if err != nil {
		return GetResult{}, fmt.Errorf("list items failed: %w", err)
	}
	sshKeys := bw.FilterSSHKeys(items)

	st, err := state.Load()
	if err != nil {
		return GetResult{}, fmt.Errorf("load state failed: %w", err)
	}
	deployer, err := ssh.NewDeployer(st)
	if err != nil {
		return GetResult{}, fmt.Errorf("create ssh deployer failed: %w", err)
	}
	existing := st.FindDeployedSSHKeysByFolder(folder)
	existingByID := make(map[string]state.SSHKeyEntry, len(existing))
	for i := range existing {
		entry := existing[i]
		existingByID[entry.ItemID] = entry
	}
	remoteByID := make(map[string]bwtypes.Item, len(sshKeys))
	for _, keyItem := range sshKeys {
		remoteByID[keyItem.ID] = keyItem
	}
	for i := range existing {
		entry := existing[i]
		if _, ok := remoteByID[entry.ItemID]; ok {
			continue
		}
		r.Infof("  Removing stale '%s'...\n", entry.KeyName)
		if err := deployer.Remove(entry); err != nil {
			return GetResult{}, fmt.Errorf("remove stale key '%s': %w", entry.KeyName, err)
		}
		st.RemoveStoredSSHKey(entry.ItemID)
	}

	deployed := 0
	authorized := 0
	for _, keyItem := range sshKeys {
		if entry, ok := existingByID[keyItem.ID]; ok && entry.KeyName != sanitizeSSHKeyName(keyItem.Name) {
			if err := deployer.Remove(entry); err != nil {
				return GetResult{}, fmt.Errorf("refresh renamed key '%s': %w", entry.KeyName, err)
			}
			st.RemoveStoredSSHKey(entry.ItemID)
		}

		r.Infof("  Deploying '%s'...\n", keyItem.Name)
		if err := deployer.Deploy(keyItem, folder); err != nil {
			r.Errorf("  Failed to deploy '%s': %v\n", keyItem.Name, err)
			continue
		}
		deployed++

		if params.AuthorizeSSH {
			if keyItem.SSHKey == nil || keyItem.SSHKey.PublicKey == "" {
				r.Warnf("  Warning: '%s' has no public key in Bitwarden; skipping authorize\n", keyItem.Name)
				continue
			}
			added, path, err := ssh.AppendAuthorizedKey(keyItem.SSHKey.PublicKey)
			if err != nil {
				r.Warnf("  Warning: authorize for '%s' failed (%s): %v\n", keyItem.Name, path, err)
				continue
			}
			if added {
				r.Infof("    Appended to %s\n", path)
				authorized++
			} else {
				r.Infof("    Already present in %s, skipped\n", path)
			}
		}
	}

	if err := deployer.RemoveAllKnownHosts(); err != nil {
		r.Warnf("Warning: known_hosts cleanup failed: %v\n", err)
	}
	if err := st.Save(); err != nil {
		return GetResult{}, fmt.Errorf("save state failed: %w", err)
	}
	if len(sshKeys) == 0 {
		r.Info("No SSH keys found in folder.")
		return GetResult{}, nil
	}
	r.Infof("Deployed %d SSH key(s).\n", deployed)
	if params.AuthorizeSSH {
		r.Infof("Authorized %d key(s) on this host.\n", authorized)
	}
	return GetResult{SSHDeployed: deployed, Authorized: authorized}, nil
}

func GetEnv(ctx context.Context, params GetParams, r Reporter) (GetResult, error) {
	folder := params.Folder
	client := bw.NewClient()

	session, err := ensureBitwardenSession(ctx, client, r)
	if err != nil {
		return GetResult{}, fmt.Errorf("authentication failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return GetResult{}, err
	}

	r.Info("Syncing vault...")
	if err := client.Sync(session); err != nil {
		return GetResult{}, fmt.Errorf("sync failed: %w", err)
	}

	r.Infof("Resolving include chain for '%s'...\n", folder)
	chain, err := client.LoadIncludeChain(session, folder)
	if err != nil {
		return GetResult{}, fmt.Errorf("resolve include chain: %w", err)
	}
	chainNames := make([]string, len(chain))
	for i, f := range chain {
		chainNames[i] = f.Name
	}
	if len(chain) > 1 {
		r.Infof("已展开 include：%s\n", strings.Join(chainNames, " -> "))
	}

	notesByFolder := make(map[string]string, len(chain))
	var currentItem bwtypes.Item
	hasCurrent := false
	for i, f := range chain {
		items, err := client.ListItems(session, f.ID)
		if err != nil {
			return GetResult{}, fmt.Errorf("list items for folder '%s': %w", f.Name, err)
		}
		envItem, found, err := bw.FindManagedEnvNote(items)
		if err != nil {
			return GetResult{}, fmt.Errorf("folder '%s': %w", f.Name, err)
		}
		if found {
			notesByFolder[f.Name] = envItem.Notes
			if i == 0 {
				currentItem = envItem
				hasCurrent = true
			}
		}
	}

	st, err := state.Load()
	if err != nil {
		return GetResult{}, fmt.Errorf("load state failed: %w", err)
	}
	deployer := env.NewDeployer(st)
	if len(notesByFolder) == 0 {
		cleaned := 0
		entries := st.FindEnvsByFolder(folder)
		for i := range entries {
			entry := entries[i]
			if err := deployer.Remove(entry); err != nil {
				return GetResult{}, err
			}
			cleaned++
		}
		st.RemoveEnvsByFolder(folder)
		if err := st.Save(); err != nil {
			return GetResult{}, fmt.Errorf("save state failed: %w", err)
		}
		if cleaned > 0 {
			r.Infof("No env note found on chain. Cleaned %d local env artifact set(s) for folder '%s'.\n", cleaned, folder)
		} else {
			r.Infof("No env note found. Create one Secure Note named '%s'.\n", bwtypes.ReservedEnvNoteName)
		}
		return GetResult{}, nil
	}

	result, err := env.MergePkvEnvNotes(chainNames, notesByFolder)
	if err != nil {
		return GetResult{}, err
	}
	if len(result.Conflicts) > 0 {
		r.Info("Env overrides:")
		for _, c := range result.Conflicts {
			r.Infof("  %s: winner=%s shadowed=%s\n", c.Key, c.Winner, strings.Join(c.Losers, ","))
		}
	}
	entry, err := deployer.DeployMerged(folder, result, currentItem, hasCurrent)
	if err != nil {
		return GetResult{}, err
	}
	if err := st.Save(); err != nil {
		return GetResult{}, fmt.Errorf("save state failed: %w", err)
	}
	if len(chain) > 1 {
		for _, v := range result.Vars {
			r.Infof("  %s [from: %s]\n", v.Key, v.Source)
		}
	}
	r.Infof("Wrote env artifacts for folder '%s'.\n", folder)
	if len(chain) > 1 {
		artifactSource := entry.SourceFolder
		if artifactSource == "" {
			artifactSource = folder
		}
		r.Infof("  [from: %s] JSON: %s\n", artifactSource, entry.JSONPath)
		r.Infof("  [from: %s] Shell: %s\n", artifactSource, entry.ShellPath)
		r.Infof("  [from: %s] PowerShell: %s\n", artifactSource, entry.PowerShellPath)
	} else {
		r.Infof("  JSON: %s\n", entry.JSONPath)
		r.Infof("  Shell: %s\n", entry.ShellPath)
		r.Infof("  PowerShell: %s\n", entry.PowerShellPath)
	}
	return GetResult{EnvKeys: len(result.Vars)}, nil
}

func GetNote(ctx context.Context, params GetParams, r Reporter) (GetResult, error) {
	folder := params.Folder
	client := bw.NewClient()

	session, err := ensureBitwardenSession(ctx, client, r)
	if err != nil {
		return GetResult{}, fmt.Errorf("authentication failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return GetResult{}, err
	}

	r.Info("Syncing vault...")
	if err := client.Sync(session); err != nil {
		return GetResult{}, fmt.Errorf("sync failed: %w", err)
	}
	chain, err := client.LoadIncludeChain(session, folder)
	if err != nil {
		return GetResult{}, err
	}
	chainNames := make([]string, len(chain))
	for i, f := range chain {
		chainNames[i] = f.Name
	}
	if len(chain) > 1 {
		r.Infof("已展开 include：%s\n", strings.Join(chainNames, " -> "))
	}
	itemsByFolder := make(map[string][]bwtypes.Item, len(chain))
	for _, f := range chain {
		items, err := client.ListItems(session, f.ID)
		if err != nil {
			return GetResult{}, fmt.Errorf("list items for folder '%s': %w", f.Name, err)
		}
		itemsByFolder[f.Name] = bw.FilterConfigNotes(items)
	}
	merged := note.MergeNoteItems(chainNames, itemsByFolder)
	if len(merged.Conflicts) > 0 {
		r.Info("Note overrides:")
		for _, c := range merged.Conflicts {
			r.Infof("  %s: winner=%s shadowed=%s\n", c.Name, c.Winner, strings.Join(c.Losers, ","))
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return GetResult{}, fmt.Errorf("get working directory failed: %w", err)
	}
	st, err := state.Load()
	if err != nil {
		return GetResult{}, fmt.Errorf("load state failed: %w", err)
	}
	notes := make([]bwtypes.Item, 0, len(merged.Items))
	sourceByID := make(map[string]string, len(merged.Items))
	for _, it := range merged.Items {
		notes = append(notes, it.Item)
		sourceByID[it.Item.ID] = it.Source
	}
	syncer := note.NewSyncer(st)
	synced, err := syncer.SyncFolderWithSources(notes, sourceByID, cwd, folder)
	if err != nil {
		return GetResult{}, err
	}
	if err := st.Save(); err != nil {
		return GetResult{}, fmt.Errorf("save state failed: %w", err)
	}
	if len(notes) == 0 {
		r.Infof("No config notes found in folder '%s'.\n", folder)
		return GetResult{}, nil
	}
	r.Infof("Synced %d note(s) to %s\n", synced, cwd)
	if len(chain) > 1 {
		for _, it := range merged.Items {
			source := sourceByID[it.Item.ID]
			if source == "" {
				source = folder
			}
			r.Infof("  [from: %s] %s\n", source, filepath.Join(cwd, it.Item.Name))
		}
	}
	return GetResult{NotesSynced: synced}, nil
}

type AddParams struct {
	Folder   string
	FolderID string
	Kind     string

	SSH     AddSSHKeyParams
	Name    string
	Content string
}

type AddResult struct {
	ItemID      string
	Fingerprint string
	Hosts       []string
}

type AddSSHKeyParams struct {
	Folder      string
	FolderID    string
	KeyName     string
	OpenSSHKey  string
	PublicKey   string
	Fingerprint string
	Hosts       []string
	Generated   bool
}

type AddFolderParams struct {
	Name string
}

type AddFolderResult struct {
	Folder bwtypes.Folder
}

type addFolderClient interface {
	EnsureUnlocked() (string, error)
	Sync(session string) error
	ListFolders(session string) ([]bwtypes.Folder, error)
	CreateFolder(session, name string) (bwtypes.Folder, error)
}

func AddFolder(ctx context.Context, params AddFolderParams, r Reporter) (AddFolderResult, error) {
	return addFolderWithClient(ctx, bw.NewClient(), params, r)
}

func addFolderWithClient(ctx context.Context, client addFolderClient, params AddFolderParams, r Reporter) (AddFolderResult, error) {
	r = reporterOrNoop(r)
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return AddFolderResult{}, fmt.Errorf("folder name is required")
	}
	if err := ctx.Err(); err != nil {
		return AddFolderResult{}, err
	}

	session, err := ensureBitwardenSession(ctx, client, r)
	if err != nil {
		return AddFolderResult{}, fmt.Errorf("authentication failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return AddFolderResult{}, err
	}

	r.Info("Syncing vault...")
	if err := client.Sync(session); err != nil {
		return AddFolderResult{}, fmt.Errorf("sync failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return AddFolderResult{}, err
	}

	folders, err := client.ListFolders(session)
	if err != nil {
		return AddFolderResult{}, fmt.Errorf("list folders failed: %w", err)
	}
	for _, folder := range folders {
		if strings.EqualFold(folder.Name, name) {
			return AddFolderResult{}, fmt.Errorf("folder '%s' already exists", folder.Name)
		}
	}

	r.Infof("Creating folder '%s'...\n", name)
	folder, err := client.CreateFolder(session, name)
	if err != nil {
		return AddFolderResult{}, fmt.Errorf("create folder failed: %w", err)
	}
	if folder.Name == "" {
		folder.Name = name
	}
	r.Infof("Folder '%s' created (ID: %s)\n", folder.Name, folder.ID)
	return AddFolderResult{Folder: folder}, nil
}

func Add(ctx context.Context, params AddParams, r Reporter) (AddResult, error) {
	switch params.Kind {
	case "ssh":
		return AddSSHKey(ctx, params.SSH, r)
	case "env":
		return AddEnv(ctx, params, r)
	case "note":
		return AddNote(ctx, params, r)
	default:
		return AddResult{}, fmt.Errorf("unknown resource type: %s (expected ssh, env, or note)", params.Kind)
	}
}

func AddSSHKey(ctx context.Context, params AddSSHKeyParams, r Reporter) (AddResult, error) {
	r = reporterOrNoop(r)
	client := bw.NewClient()
	session, err := ensureBitwardenSession(ctx, client, r)
	if err != nil {
		return AddResult{}, fmt.Errorf("authentication failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return AddResult{}, err
	}
	r.Info("Syncing vault...")
	if err := client.Sync(session); err != nil {
		return AddResult{}, fmt.Errorf("sync failed: %w", err)
	}
	r.Infof("Looking up folder '%s'...\n", params.Folder)
	folderID, err := resolveFolderID(client, session, params.Folder, params.FolderID)
	if err != nil {
		return AddResult{}, fmt.Errorf("folder lookup failed: %w", err)
	}
	notes := strings.Join(params.Hosts, "\n")
	r.Info("Creating SSH key in Bitwarden...")
	itemID, err := key.CreateBWSSHKey(client, session, params.KeyName, folderID, notes, params.OpenSSHKey, params.PublicKey, params.Fingerprint)
	if err != nil {
		return AddResult{}, fmt.Errorf("create SSH key failed: %w", err)
	}
	st, err := state.Load()
	if err != nil {
		return AddResult{}, fmt.Errorf("load state failed: %w", err)
	}
	st.AddStoredSSHKey(itemID, params.KeyName, params.Fingerprint)
	if err := st.Save(); err != nil {
		return AddResult{}, fmt.Errorf("save state failed: %w", err)
	}
	r.Infof("\nSSH key '%s' added to folder '%s'\n", params.KeyName, params.Folder)
	r.Infof("  Fingerprint: %s\n", params.Fingerprint)
	if params.Generated {
		if len(params.Hosts) > 0 {
			r.Infof("  Hosts: %s\n", strings.Join(params.Hosts, ", "))
		}
		r.Infof("  Public key: %s\n", params.PublicKey)
	}
	return AddResult{ItemID: itemID, Fingerprint: params.Fingerprint, Hosts: params.Hosts}, nil
}

func AddEnv(ctx context.Context, params AddParams, r Reporter) (AddResult, error) {
	r = reporterOrNoop(r)
	client := bw.NewClient()
	session, err := ensureBitwardenSession(ctx, client, r)
	if err != nil {
		return AddResult{}, fmt.Errorf("authentication failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return AddResult{}, err
	}
	r.Info("Syncing vault...")
	if err := client.Sync(session); err != nil {
		return AddResult{}, fmt.Errorf("sync failed: %w", err)
	}
	r.Infof("Looking up folder '%s'...\n", params.Folder)
	folderID, err := resolveFolderID(client, session, params.Folder, params.FolderID)
	if err != nil {
		return AddResult{}, fmt.Errorf("folder lookup failed: %w", err)
	}
	items, err := client.ListItems(session, folderID)
	if err != nil {
		return AddResult{}, fmt.Errorf("list items failed: %w", err)
	}
	existing, found, err := bw.FindManagedEnvNote(items)
	if err != nil {
		return AddResult{}, err
	}
	if found {
		r.Infof("Updating env note '%s'...\n", existing.Name)
		if err := securenote.UpdateContent(client, session, existing.ID, params.Content); err != nil {
			return AddResult{}, err
		}
		r.Infof("Env note '%s' updated.\n", existing.Name)
		return AddResult{ItemID: existing.ID}, nil
	}
	r.Infof("Creating env note '%s'...\n", bwtypes.ReservedEnvNoteName)
	itemID, err := securenote.Add(client, session, folderID, bwtypes.ReservedEnvNoteName, params.Content)
	if err != nil {
		return AddResult{}, fmt.Errorf("create env note failed: %w", err)
	}
	r.Infof("Env note '%s' created (ID: %s)\n", bwtypes.ReservedEnvNoteName, itemID)
	return AddResult{ItemID: itemID}, nil
}

func AddNote(ctx context.Context, params AddParams, r Reporter) (AddResult, error) {
	r = reporterOrNoop(r)
	if params.Name == "" {
		return AddResult{}, fmt.Errorf("note name is required")
	}
	if params.Name == bwtypes.ReservedEnvNoteName {
		return AddResult{}, fmt.Errorf("note name '%s' is reserved for folder env data", bwtypes.ReservedEnvNoteName)
	}
	client := bw.NewClient()
	session, err := ensureBitwardenSession(ctx, client, r)
	if err != nil {
		return AddResult{}, fmt.Errorf("authentication failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return AddResult{}, err
	}
	r.Info("Syncing vault...")
	if err := client.Sync(session); err != nil {
		return AddResult{}, fmt.Errorf("sync failed: %w", err)
	}
	r.Infof("Looking up folder '%s'...\n", params.Folder)
	folderID, err := resolveFolderID(client, session, params.Folder, params.FolderID)
	if err != nil {
		return AddResult{}, fmt.Errorf("folder lookup failed: %w", err)
	}
	r.Infof("Creating note '%s'...\n", params.Name)
	itemID, err := securenote.Add(client, session, folderID, params.Name, params.Content)
	if err != nil {
		return AddResult{}, fmt.Errorf("create note failed: %w", err)
	}
	r.Infof("Note '%s' created (ID: %s)\n", params.Name, itemID)
	return AddResult{ItemID: itemID}, nil
}

type EditSecureNoteFunc func(client *bw.Client, session string, item bwtypes.Item) (bool, error)

type EditParams struct {
	Folder   string
	FolderID string
	Kind     string
	NameOrID string
	EditNote EditSecureNoteFunc
}

type EditResult struct {
	Updated bool
	ItemID  string
	Name    string
}

func Edit(ctx context.Context, params EditParams, r Reporter) (EditResult, error) {
	switch params.Kind {
	case "env":
		return EditEnv(ctx, params, r)
	case "note":
		return EditNote(ctx, params, r)
	default:
		return EditResult{}, fmt.Errorf("unknown resource type: %s (expected env or note)", params.Kind)
	}
}

func EditEnv(ctx context.Context, params EditParams, r Reporter) (EditResult, error) {
	r = reporterOrNoop(r)
	editFn := params.EditNote
	if editFn == nil {
		editFn = securenote.Edit
	}
	client := bw.NewClient()
	session, err := ensureBitwardenSession(ctx, client, r)
	if err != nil {
		return EditResult{}, fmt.Errorf("authentication failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return EditResult{}, err
	}
	r.Info("Syncing vault...")
	if err := client.Sync(session); err != nil {
		return EditResult{}, fmt.Errorf("sync failed: %w", err)
	}
	r.Infof("Looking up folder '%s'...\n", params.Folder)
	folderID, err := resolveFolderID(client, session, params.Folder, params.FolderID)
	if err != nil {
		return EditResult{}, fmt.Errorf("folder lookup failed: %w", err)
	}
	items, err := client.ListItems(session, folderID)
	if err != nil {
		return EditResult{}, fmt.Errorf("list items failed: %w", err)
	}
	item, found, err := bw.FindManagedEnvNote(items)
	if err != nil {
		return EditResult{}, err
	}
	if !found {
		return EditResult{}, fmt.Errorf("no env note found in folder '%s' (expected Secure Note named '%s')", params.Folder, bwtypes.ReservedEnvNoteName)
	}
	r.Infof("Editing '%s'...\n", item.Name)
	updated, err := editFn(client, session, item)
	if err != nil {
		return EditResult{}, fmt.Errorf("edit failed: %w", err)
	}
	if !updated {
		r.Info("No changes made.")
		return EditResult{ItemID: item.ID, Name: item.Name}, nil
	}
	r.Infof("Env note '%s' updated.\n", item.Name)
	return EditResult{Updated: true, ItemID: item.ID, Name: item.Name}, nil
}

func EditNote(ctx context.Context, params EditParams, r Reporter) (EditResult, error) {
	r = reporterOrNoop(r)
	editFn := params.EditNote
	if editFn == nil {
		editFn = securenote.Edit
	}
	client := bw.NewClient()
	session, err := ensureBitwardenSession(ctx, client, r)
	if err != nil {
		return EditResult{}, fmt.Errorf("authentication failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return EditResult{}, err
	}
	r.Info("Syncing vault...")
	if err := client.Sync(session); err != nil {
		return EditResult{}, fmt.Errorf("sync failed: %w", err)
	}
	r.Infof("Looking up folder '%s'...\n", params.Folder)
	folderID, err := resolveFolderID(client, session, params.Folder, params.FolderID)
	if err != nil {
		return EditResult{}, fmt.Errorf("folder lookup failed: %w", err)
	}
	items, err := client.ListItems(session, folderID)
	if err != nil {
		return EditResult{}, fmt.Errorf("list items failed: %w", err)
	}
	item, err := securenote.ResolveItem(bw.FilterConfigNotes(items), params.NameOrID)
	if err != nil {
		return EditResult{}, err
	}
	r.Infof("Editing '%s'...\n", item.Name)
	updated, err := editFn(client, session, item)
	if err != nil {
		return EditResult{}, fmt.Errorf("edit failed: %w", err)
	}
	if !updated {
		r.Info("No changes made.")
		return EditResult{ItemID: item.ID, Name: item.Name}, nil
	}
	r.Infof("Note '%s' updated.\n", item.Name)
	return EditResult{Updated: true, ItemID: item.ID, Name: item.Name}, nil
}

type RemoveParams struct {
	Folder   string
	FolderID string
	Kind     string
	IDs      []string
}

type RemoveResult struct{ Removed int }

func Remove(ctx context.Context, params RemoveParams, r Reporter) (RemoveResult, error) {
	switch params.Kind {
	case "ssh":
		return RemoveSSH(ctx, params, r)
	case "env":
		return RemoveEnv(ctx, params, r)
	case "note":
		return RemoveNote(ctx, params, r)
	default:
		return RemoveResult{}, fmt.Errorf("unknown resource type: %s (expected ssh, env, or note)", params.Kind)
	}
}

func RemoveSSH(ctx context.Context, params RemoveParams, r Reporter) (RemoveResult, error) {
	r = reporterOrNoop(r)
	client := bw.NewClient()
	session, err := ensureBitwardenSession(ctx, client, r)
	if err != nil {
		return RemoveResult{}, fmt.Errorf("authentication failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return RemoveResult{}, err
	}
	r.Info("Syncing vault...")
	if err := client.Sync(session); err != nil {
		return RemoveResult{}, fmt.Errorf("sync failed: %w", err)
	}
	r.Infof("Looking up folder '%s'...\n", params.Folder)
	folderID, err := resolveFolderID(client, session, params.Folder, params.FolderID)
	if err != nil {
		return RemoveResult{}, fmt.Errorf("folder lookup failed: %w", err)
	}
	items, err := client.ListItems(session, folderID)
	if err != nil {
		return RemoveResult{}, fmt.Errorf("list items failed: %w", err)
	}
	sshKeys := bw.FilterSSHKeys(items)
	keyMap := make(map[string]string)
	for _, item := range sshKeys {
		keyMap[item.ID] = item.Name
	}
	st, err := state.Load()
	if err != nil {
		return RemoveResult{}, fmt.Errorf("load state failed: %w", err)
	}
	deployer, err := ssh.NewDeployer(st)
	if err != nil {
		return RemoveResult{}, fmt.Errorf("create ssh deployer failed: %w", err)
	}
	deployedByID := make(map[string]state.SSHKeyEntry)
	deployedEntries := st.FindDeployedSSHKeysByFolder(params.Folder)
	for i := range deployedEntries {
		entry := deployedEntries[i]
		deployedByID[entry.ItemID] = entry
	}
	r.Infof("Removing SSH keys from folder '%s'...\n", params.Folder)
	removed := 0
	for _, id := range params.IDs {
		name, found := keyMap[id]
		if !found {
			r.Errorf("  Key '%s' not found in folder '%s'\n", id, params.Folder)
			continue
		}
		if err := client.DeleteItem(session, id); err != nil {
			r.Errorf("  Failed to remove '%s' (%s): %v\n", name, id, err)
			continue
		}
		cleanupFailed := false
		if entry, ok := deployedByID[id]; ok {
			if err := deployer.Remove(entry); err != nil {
				r.Errorf("  Failed to clean local '%s': %v\n", name, err)
				cleanupFailed = true
			}
		}
		if !cleanupFailed {
			st.RemoveStoredSSHKey(id)
		}
		r.Infof("  Removed '%s' (%s)\n", name, id)
		removed++
	}
	if err := deployer.RemoveAllKnownHosts(); err != nil {
		r.Warnf("  Warning: known_hosts cleanup failed: %v\n", err)
	}
	if err := st.Save(); err != nil {
		return RemoveResult{}, fmt.Errorf("save state failed: %w", err)
	}
	r.Infof("Removed %d SSH key(s).\n", removed)
	return RemoveResult{Removed: removed}, nil
}

func RemoveEnv(ctx context.Context, params RemoveParams, r Reporter) (RemoveResult, error) {
	r = reporterOrNoop(r)
	client := bw.NewClient()
	session, err := ensureBitwardenSession(ctx, client, r)
	if err != nil {
		return RemoveResult{}, fmt.Errorf("authentication failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return RemoveResult{}, err
	}
	r.Info("Syncing vault...")
	if err := client.Sync(session); err != nil {
		return RemoveResult{}, fmt.Errorf("sync failed: %w", err)
	}
	r.Infof("Looking up folder '%s'...\n", params.Folder)
	folderID, err := resolveFolderID(client, session, params.Folder, params.FolderID)
	if err != nil {
		return RemoveResult{}, fmt.Errorf("folder lookup failed: %w", err)
	}
	items, err := client.ListItems(session, folderID)
	if err != nil {
		return RemoveResult{}, fmt.Errorf("list items failed: %w", err)
	}
	item, found, err := bw.FindManagedEnvNote(items)
	if err != nil {
		return RemoveResult{}, err
	}
	if !found {
		r.Infof("No env note found in folder '%s'.\n", params.Folder)
		return RemoveResult{}, nil
	}
	if err := client.DeleteItem(session, item.ID); err != nil {
		return RemoveResult{}, fmt.Errorf("remove env note failed: %w", err)
	}
	st, err := state.Load()
	if err != nil {
		return RemoveResult{}, fmt.Errorf("load state failed: %w", err)
	}
	deployer := env.NewDeployer(st)
	entries := st.FindEnvsByFolder(params.Folder)
	cleanupFailed := false
	for i := range entries {
		entry := entries[i]
		if err := deployer.Remove(entry); err != nil {
			r.Errorf("  Failed to clean local env artifacts for '%s': %v\n", entry.Name, err)
			cleanupFailed = true
		}
	}
	if !cleanupFailed {
		st.RemoveEnvsByFolder(params.Folder)
	}
	if err := st.Save(); err != nil {
		return RemoveResult{}, fmt.Errorf("save state failed: %w", err)
	}
	r.Infof("Removed env note '%s' (%s).\n", item.Name, item.ID)
	return RemoveResult{Removed: 1}, nil
}

func RemoveNote(ctx context.Context, params RemoveParams, r Reporter) (RemoveResult, error) {
	r = reporterOrNoop(r)
	client := bw.NewClient()
	session, err := ensureBitwardenSession(ctx, client, r)
	if err != nil {
		return RemoveResult{}, fmt.Errorf("authentication failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return RemoveResult{}, err
	}
	r.Info("Syncing vault...")
	if err := client.Sync(session); err != nil {
		return RemoveResult{}, fmt.Errorf("sync failed: %w", err)
	}
	r.Infof("Looking up folder '%s'...\n", params.Folder)
	folderID, err := resolveFolderID(client, session, params.Folder, params.FolderID)
	if err != nil {
		return RemoveResult{}, fmt.Errorf("folder lookup failed: %w", err)
	}
	items, err := client.ListItems(session, folderID)
	if err != nil {
		return RemoveResult{}, fmt.Errorf("list items failed: %w", err)
	}
	notes := bw.FilterConfigNotes(items)
	noteMap := make(map[string]string)
	for _, item := range notes {
		noteMap[item.ID] = item.Name
	}
	st, err := state.Load()
	if err != nil {
		return RemoveResult{}, fmt.Errorf("load state failed: %w", err)
	}
	syncer := note.NewSyncer(st)
	r.Infof("Removing notes from folder '%s'...\n", params.Folder)
	removed := 0
	for _, id := range params.IDs {
		name, found := noteMap[id]
		if !found {
			r.Errorf("  Note '%s' not found in folder '%s'\n", id, params.Folder)
			continue
		}
		if err := client.DeleteItem(session, id); err != nil {
			r.Errorf("  Failed to remove '%s' (%s): %v\n", name, id, err)
			continue
		}
		cleanupFailed := false
		for i := range st.Notes {
			entry := st.Notes[i]
			if entry.ItemID != id {
				continue
			}
			if err := syncer.Remove(entry); err != nil {
				r.Errorf("  Failed to clean local '%s': %v\n", entry.FilePath, err)
				cleanupFailed = true
			}
		}
		if !cleanupFailed {
			st.RemoveNote(id)
		}
		r.Infof("  Removed '%s' (%s)\n", name, id)
		removed++
	}
	if err := st.Save(); err != nil {
		return RemoveResult{}, fmt.Errorf("save state failed: %w", err)
	}
	r.Infof("Removed %d note(s).\n", removed)
	return RemoveResult{Removed: removed}, nil
}

type CleanParams struct {
	Folder   string
	FolderID string
	Kind     string
}

type CleanResult struct{ Cleaned int }

func Clean(ctx context.Context, params CleanParams, r Reporter) (CleanResult, error) {
	switch params.Kind {
	case "ssh":
		return CleanSSH(ctx, params, r)
	case "env":
		return CleanEnv(ctx, params, r)
	case "note":
		return CleanNote(ctx, params, r)
	default:
		return CleanResult{}, fmt.Errorf("unknown resource type: %s (expected ssh, env, or note)", params.Kind)
	}
}

func CleanSSH(ctx context.Context, params CleanParams, r Reporter) (CleanResult, error) {
	r = reporterOrNoop(r)
	if err := ctx.Err(); err != nil {
		return CleanResult{}, err
	}
	st, err := state.Load()
	if err != nil {
		return CleanResult{}, fmt.Errorf("load state failed: %w", err)
	}
	entries := st.FindDeployedSSHKeysByFolder(params.Folder)
	if len(entries) == 0 {
		r.Infof("No SSH keys found for folder '%s'.\n", params.Folder)
		return CleanResult{}, nil
	}
	deployer, err := ssh.NewDeployer(st)
	if err != nil {
		return CleanResult{}, fmt.Errorf("create ssh deployer failed: %w", err)
	}
	cleaned := 0
	for i := range entries {
		entry := entries[i]
		r.Infof("  Removing '%s'...\n", entry.KeyName)
		if err := deployer.Remove(entry); err != nil {
			r.Errorf("  Failed to remove '%s': %v\n", entry.KeyName, err)
			continue
		}
		st.RemoveStoredSSHKey(entry.ItemID)
		cleaned++
	}
	if err := deployer.RemoveAllKnownHosts(); err != nil {
		r.Warnf("  Warning: known_hosts cleanup failed: %v\n", err)
	}
	if err := st.Save(); err != nil {
		return CleanResult{}, fmt.Errorf("save state failed: %w", err)
	}
	r.Infof("Cleaned %d SSH key(s) for folder '%s'.\n", cleaned, params.Folder)
	return CleanResult{Cleaned: cleaned}, nil
}

func CleanEnv(ctx context.Context, params CleanParams, r Reporter) (CleanResult, error) {
	r = reporterOrNoop(r)
	if err := ctx.Err(); err != nil {
		return CleanResult{}, err
	}
	st, err := state.Load()
	if err != nil {
		return CleanResult{}, fmt.Errorf("load state failed: %w", err)
	}
	entries := st.FindEnvsByFolder(params.Folder)
	if len(entries) == 0 {
		r.Infof("No env artifacts found for folder '%s'.\n", params.Folder)
		return CleanResult{}, nil
	}
	deployer := env.NewDeployer(st)
	cleaned := 0
	for i := range entries {
		entry := entries[i]
		r.Infof("  Removing env artifacts for '%s'...\n", entry.Name)
		if err := deployer.Remove(entry); err != nil {
			r.Errorf("  Failed to remove '%s': %v\n", entry.Name, err)
			continue
		}
		cleaned++
	}
	if cleaned == len(entries) {
		st.RemoveEnvsByFolder(params.Folder)
	} else {
		r.Warn("Some env artifacts could not be removed; state was kept so you can retry clean.")
	}
	if err := st.Save(); err != nil {
		return CleanResult{}, fmt.Errorf("save state failed: %w", err)
	}
	r.Infof("Cleaned %d env artifact set(s) for '%s'.\n", cleaned, params.Folder)
	return CleanResult{Cleaned: cleaned}, nil
}

func CleanNote(ctx context.Context, params CleanParams, r Reporter) (CleanResult, error) {
	r = reporterOrNoop(r)
	if err := ctx.Err(); err != nil {
		return CleanResult{}, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return CleanResult{}, fmt.Errorf("get working directory failed: %w", err)
	}
	if absDir, err := filepath.Abs(cwd); err == nil {
		cwd = absDir
	}
	st, err := state.Load()
	if err != nil {
		return CleanResult{}, fmt.Errorf("load state failed: %w", err)
	}
	entries := st.FindSyncedNotes(params.Folder, cwd)
	if len(entries) == 0 {
		r.Infof("No synced notes found for folder '%s' in %s.\n", params.Folder, cwd)
		return CleanResult{}, nil
	}
	syncer := note.NewSyncer(st)
	cleaned := 0
	for i := range entries {
		entry := entries[i]
		r.Infof("  Removing '%s'...\n", entry.FileName)
		if err := syncer.Remove(entry); err != nil {
			r.Errorf("  Failed to remove '%s': %v\n", entry.FileName, err)
			continue
		}
		st.RemoveNoteForTarget(entry.ItemID, params.Folder, cwd)
		cleaned++
	}
	if err := st.Save(); err != nil {
		return CleanResult{}, fmt.Errorf("save state failed: %w", err)
	}
	r.Infof("Cleaned %d note(s) for folder '%s' in %s.\n", cleaned, params.Folder, cwd)
	return CleanResult{Cleaned: cleaned}, nil
}

func resolveFolderID(client *bw.Client, session, folderName, folderID string) (string, error) {
	if strings.TrimSpace(folderID) != "" {
		return folderID, nil
	}
	return client.GetFolderID(session, folderName)
}

func sanitizeSSHKeyName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('_')
		}
	}
	return b.String()
}
