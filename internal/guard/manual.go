package guard

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shichao402/pkv/internal/app"
	"github.com/shichao402/pkv/internal/bw/types"
	"github.com/shichao402/pkv/internal/note"
	"github.com/shichao402/pkv/internal/state"
)

// TransferOptions selects notes for manual download or upload.
// Note may be an item_id, file name, empty, or "all".
// WorkspaceID limits to one registered workspace root_path.
type TransferOptions struct {
	Note        string
	WorkspaceID string
}

type TransferResult struct {
	ItemID   string `json:"item_id"`
	FileName string `json:"file_name"`
	FilePath string `json:"file_path,omitempty"`
	Action   string `json:"action"`
	Backup   string `json:"backup,omitempty"`
	Error    string `json:"error,omitempty"`
}

type TransferSummary struct {
	Results    []TransferResult `json:"results"`
	Downloaded int              `json:"downloaded"`
	Uploaded   int              `json:"uploaded"`
	Skipped    int              `json:"skipped"`
	Errors     int              `json:"errors"`
}

// Download pulls note content from Bitwarden into local tracked files.
// Existing local content is backed up to ~/.pkv/backups/<item_id>/ before overwrite.
func (g *Guard) Download(ctx context.Context, opts TransferOptions) (TransferSummary, error) {
	summary := TransferSummary{}
	if !g.resolveSession(ctx) {
		return summary, fmt.Errorf("no valid Bitwarden session; %s", app.NeedsUnlockMessage())
	}

	g.mu.Lock()
	st := g.state
	session := g.session
	g.mu.Unlock()

	workspaces, err := g.resolveWorkspaces(st, opts.WorkspaceID)
	if err != nil {
		return summary, err
	}
	if len(workspaces) == 0 {
		return summary, fmt.Errorf("no registered workspaces")
	}

	if err := g.client.Sync(session); err != nil {
		return summary, fmt.Errorf("sync vault: %w", err)
	}

	allNotes := strings.EqualFold(strings.TrimSpace(opts.Note), "all") || strings.TrimSpace(opts.Note) == ""

	for _, ws := range workspaces {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		if allNotes {
			newCount, err := g.downloadNewRemoteNotes(ctx, st, session, ws, &summary)
			if err != nil {
				return summary, err
			}
			summary.Downloaded += newCount
		}

		entries, err := g.selectTrackedNotes(st, ws, opts.Note)
		if err != nil {
			return summary, err
		}
		for _, entry := range entries {
			result := g.downloadOne(ctx, st, session, ws, entry)
			summary.Results = append(summary.Results, result)
			switch result.Action {
			case "downloaded":
				summary.Downloaded++
			case "skipped":
				summary.Skipped++
			default:
				if result.Error != "" {
					summary.Errors++
				}
			}
		}
	}

	if err := st.Save(); err != nil {
		return summary, fmt.Errorf("save state: %w", err)
	}
	return summary, nil
}

// Upload pushes local tracked note files to Bitwarden.
// Remote content is backed up to ~/.pkv/backups/<item_id>/ before overwrite.
func (g *Guard) Upload(ctx context.Context, opts TransferOptions) (TransferSummary, error) {
	summary := TransferSummary{}
	if !g.resolveSession(ctx) {
		return summary, fmt.Errorf("no valid Bitwarden session; %s", app.NeedsUnlockMessage())
	}

	g.mu.Lock()
	st := g.state
	session := g.session
	g.mu.Unlock()

	workspaces, err := g.resolveWorkspaces(st, opts.WorkspaceID)
	if err != nil {
		return summary, err
	}
	if len(workspaces) == 0 {
		return summary, fmt.Errorf("no registered workspaces")
	}

	for _, ws := range workspaces {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		entries, err := g.selectTrackedNotes(st, ws, opts.Note)
		if err != nil {
			return summary, err
		}
		for _, entry := range entries {
			result := g.uploadOne(ctx, st, session, ws, entry)
			summary.Results = append(summary.Results, result)
			switch result.Action {
			case "uploaded":
				summary.Uploaded++
			case "skipped":
				summary.Skipped++
			default:
				if result.Error != "" {
					summary.Errors++
				}
			}
		}
	}

	if err := st.Save(); err != nil {
		return summary, fmt.Errorf("save state: %w", err)
	}
	return summary, nil
}

func (g *Guard) resolveWorkspaces(st *state.State, workspaceID string) ([]state.WorkspaceEntry, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return st.ListWorkspaces(), nil
	}
	ws, err := FindWorkspaceByID(st, workspaceID)
	if err != nil {
		return nil, err
	}
	return []state.WorkspaceEntry{*ws}, nil
}

func (g *Guard) selectTrackedNotes(st *state.State, ws state.WorkspaceEntry, noteRef string) ([]state.NoteEntry, error) {
	noteRef = strings.TrimSpace(noteRef)
	if noteRef == "" || strings.EqualFold(noteRef, "all") {
		return st.FindSyncedNotes(ws.Folder, ws.TargetDir), nil
	}
	for _, entry := range st.FindSyncedNotes(ws.Folder, ws.TargetDir) {
		if entry.ItemID == noteRef || entry.FileName == noteRef {
			return []state.NoteEntry{entry}, nil
		}
	}
	return nil, fmt.Errorf("tracked note not found: %q in workspace %s", noteRef, ws.RootPath)
}

func (g *Guard) downloadNewRemoteNotes(ctx context.Context, st *state.State, session string, ws state.WorkspaceEntry, summary *TransferSummary) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	notes, sourceByID, err := g.loadMergedNotes(session, ws.Folder)
	if err != nil {
		return 0, err
	}
	tracked := make(map[string]struct{})
	for _, entry := range st.FindSyncedNotes(ws.Folder, ws.TargetDir) {
		tracked[entry.ItemID] = struct{}{}
	}
	var newNotes []types.Item
	newSources := make(map[string]string)
	for _, item := range notes {
		if _, ok := tracked[item.ID]; ok {
			continue
		}
		newNotes = append(newNotes, item)
		if src, ok := sourceByID[item.ID]; ok {
			newSources[item.ID] = src
		}
	}
	if len(newNotes) == 0 {
		return 0, nil
	}
	syncer := note.NewSyncer(st)
	count, err := syncer.SyncFolderWithSources(newNotes, newSources, ws.TargetDir, ws.Folder)
	if err != nil {
		return 0, err
	}
	for _, item := range newNotes {
		entry := st.FindNoteEntry(item.ID, ws.Folder, ws.TargetDir)
		if entry == nil {
			continue
		}
		summary.Results = append(summary.Results, TransferResult{
			ItemID:   item.ID,
			FileName: item.Name,
			FilePath: entry.FilePath,
			Action:   "downloaded",
		})
	}
	return count, nil
}

func (g *Guard) downloadOne(ctx context.Context, st *state.State, session string, ws state.WorkspaceEntry, entry state.NoteEntry) TransferResult {
	result := TransferResult{
		ItemID:   entry.ItemID,
		FileName: entry.FileName,
		FilePath: entry.FilePath,
	}
	if err := ctx.Err(); err != nil {
		result.Action = "error"
		result.Error = err.Error()
		return result
	}

	remote, err := g.client.GetItem(session, entry.ItemID)
	if err != nil {
		result.Action = "error"
		result.Error = err.Error()
		return result
	}
	if remote.Notes == "" {
		result.Action = "skipped"
		return result
	}

	localContent, _, err := readLocalNote(entry.FilePath)
	if err != nil {
		result.Action = "error"
		result.Error = err.Error()
		return result
	}
	if localContent == remote.Notes {
		result.Action = "skipped"
		return result
	}

	if localContent != "" {
		backupPath, err := BackupNoteContent(entry.ItemID, "local", entry.FileName, []byte(localContent))
		if err != nil {
			result.Action = "error"
			result.Error = fmt.Sprintf("backup local: %v", err)
			return result
		}
		result.Backup = backupPath
	}

	out, err := ReconcileNote(ReconcileInput{
		Entry:         entry,
		Remote:        remote,
		LocalContent:  localContent,
		WorkspaceRoot: ws.RootPath,
	}, ReconcileDecision{Action: ActionPullRemote}, nil)
	if err != nil {
		result.Action = "error"
		result.Error = err.Error()
		return result
	}
	st.UpsertNote(out.UpdatedEntry)
	result.Action = "downloaded"
	result.FilePath = out.WrittenPath
	return result
}

func (g *Guard) uploadOne(ctx context.Context, st *state.State, session string, ws state.WorkspaceEntry, entry state.NoteEntry) TransferResult {
	result := TransferResult{
		ItemID:   entry.ItemID,
		FileName: entry.FileName,
		FilePath: entry.FilePath,
	}
	if err := ctx.Err(); err != nil {
		result.Action = "error"
		result.Error = err.Error()
		return result
	}

	localContent, localMod, err := readLocalNote(entry.FilePath)
	if err != nil {
		result.Action = "error"
		result.Error = err.Error()
		return result
	}
	if localContent == "" {
		result.Action = "skipped"
		return result
	}

	remote, err := g.client.GetItem(session, entry.ItemID)
	if err != nil {
		result.Action = "error"
		result.Error = err.Error()
		return result
	}
	if remote.Notes == localContent {
		result.Action = "skipped"
		return result
	}

	if remote.Notes != "" {
		backupPath, err := BackupNoteContent(entry.ItemID, "remote", entry.FileName, []byte(remote.Notes))
		if err != nil {
			result.Action = "error"
			result.Error = fmt.Sprintf("backup remote: %v", err)
			return result
		}
		result.Backup = backupPath
	}

	pusher := BWNotePusher{Client: g.client, Session: session}
	out, err := ReconcileNote(ReconcileInput{
		Entry:         entry,
		Remote:        remote,
		LocalContent:  localContent,
		LocalModTime:  localMod,
		WorkspaceRoot: ws.RootPath,
	}, ReconcileDecision{Action: ActionPushLocal}, pusher)
	if err != nil {
		result.Action = "error"
		result.Error = err.Error()
		return result
	}

	if err := g.client.Sync(session); err != nil {
		result.Action = "error"
		result.Error = fmt.Sprintf("bw sync after push: %v", err)
		return result
	}
	if refreshed, err := g.client.GetItem(session, entry.ItemID); err == nil {
		if rev, err := refreshed.RevisionTime(); err == nil {
			out.UpdatedEntry.RemoteRevisionAt = formatStateTime(rev)
		}
	}

	st.UpsertNote(out.UpdatedEntry)
	result.Action = "uploaded"
	return result
}

func readLocalNote(path string) (content string, modTime time.Time, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", time.Time{}, nil
		}
		return "", time.Time{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return string(data), time.Time{}, nil
	}
	return string(data), info.ModTime().UTC(), nil
}
