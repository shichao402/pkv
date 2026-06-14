package guard

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/shichao402/pkv/internal/app"
	"github.com/shichao402/pkv/internal/bw"
	"github.com/shichao402/pkv/internal/bw/types"
	"github.com/shichao402/pkv/internal/note"
	"github.com/shichao402/pkv/internal/state"
)

const defaultPollInterval = 30 * time.Second

type Status struct {
	SessionPresent bool   `json:"session_present"`
	SessionSource  string `json:"session_source"`
	SessionMissing bool   `json:"session_missing"`
	WatchRunning   bool   `json:"watch_running"`
	LastSyncError  string `json:"last_sync_error,omitempty"`
	NeedsUnlock    string `json:"needs_unlock,omitempty"`
}

type ConflictNotifier func(notes []state.NoteEntry)

type Guard struct {
	state    *state.State
	client   *bw.Client
	session  string
	pollEvery time.Duration

	mu             sync.Mutex
	watcher        *Watcher
	stop           context.CancelFunc
	sessionSource  app.SessionSource
	sessionMissing bool
	watchRunning   bool
	lastSyncError  string
	onConflict     ConflictNotifier
}

type SyncSummary struct {
	Workspace string
	Reconciled int
	Conflicts  int
	Skipped    int
}

type RegisterWorkspaceResult struct {
	Entry              state.WorkspaceEntry `json:"entry"`
	AlreadyRegistered  bool                 `json:"already_registered,omitempty"`
	FolderValidated    bool                 `json:"folder_validated,omitempty"`
	BootstrapSkipped   bool                 `json:"bootstrap_skipped,omitempty"`
	BootstrapNotes     int                  `json:"bootstrap_notes,omitempty"`
}

func New(st *state.State, client *bw.Client, session string) *Guard {
	if client == nil {
		client = bw.NewClient()
	}
	return &Guard{
		state:          st,
		client:         client,
		session:        session,
		pollEvery:      defaultPollInterval,
		sessionMissing: strings.TrimSpace(session) == "",
		sessionSource:  app.SessionSourceNone,
	}
}

func (g *Guard) SetSession(session string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.session = session
	if session == "" {
		g.sessionSource = app.SessionSourceNone
		g.sessionMissing = true
		return
	}
	g.sessionSource = app.SessionSourceMemory
	g.sessionMissing = false
}

func (g *Guard) Session() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.session
}

func (g *Guard) ApplyResolvedSession(resolved app.ResolvedSession) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !resolved.Valid {
		g.session = ""
		g.sessionSource = app.SessionSourceNone
		g.sessionMissing = true
		return
	}
	g.session = resolved.Session
	g.sessionSource = resolved.Source
	g.sessionMissing = false
}

func (g *Guard) SetConflictNotifier(fn ConflictNotifier) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.onConflict = fn
}

func (g *Guard) notifyConflicts(notes []state.NoteEntry) {
	g.mu.Lock()
	fn := g.onConflict
	g.mu.Unlock()
	if fn != nil && len(notes) > 0 {
		fn(notes)
	}
}

func (g *Guard) Status() Status {
	g.mu.Lock()
	defer g.mu.Unlock()
	status := Status{
		SessionPresent: g.session != "" && !g.sessionMissing,
		SessionSource:  string(g.sessionSource),
		SessionMissing: g.sessionMissing,
		WatchRunning:   g.watchRunning,
		LastSyncError:  g.lastSyncError,
	}
	if status.SessionMissing {
		status.NeedsUnlock = app.NeedsUnlockMessage()
	}
	if status.SessionSource == "" {
		status.SessionSource = string(app.SessionSourceNone)
	}
	return status
}

func (g *Guard) Start(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stop != nil {
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	g.stop = cancel
	g.watcher = NewWatcher(g.onLocalChange)
	for _, ws := range g.state.ListWorkspaces() {
		_ = g.watcher.AddWorkspace(ws.RootPath)
	}
	if err := g.watcher.Start(runCtx); err != nil {
		cancel()
		g.stop = nil
		g.watcher = nil
		return err
	}
	g.watchRunning = true
	go func() {
		_ = g.resolveSession(runCtx)
		g.pollLoop(runCtx)
	}()
	return nil
}

func (g *Guard) Stop() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stop != nil {
		g.stop()
		g.stop = nil
	}
	if g.watcher != nil {
		g.watcher.Stop()
		g.watcher = nil
	}
	g.watchRunning = false
}

// RegisterWorkspace validates, persists, watches, and bootstraps a workspace.
func (g *Guard) RegisterWorkspace(ctx context.Context, rootPath, folder, targetDir string) (RegisterWorkspaceResult, error) {
	g.mu.Lock()
	st := g.state
	g.mu.Unlock()

	entry, alreadyRegistered, err := RegisterWorkspace(st, rootPath, folder, targetDir)
	if err != nil {
		return RegisterWorkspaceResult{}, err
	}

	result := RegisterWorkspaceResult{
		Entry:             entry,
		AlreadyRegistered: alreadyRegistered,
	}

	if g.resolveSession(ctx) {
		g.mu.Lock()
		session := g.session
		g.mu.Unlock()

		folders, err := g.client.ListFolders(session)
		if err != nil {
			g.setLastSyncError(fmt.Sprintf("folder lookup: %v", err))
			return result, fmt.Errorf("list Bitwarden folders: %w", err)
		}
		if !folderExists(folders, folder) {
			return result, fmt.Errorf("Bitwarden folder not found: %s", folder)
		}
		result.FolderValidated = true

		if !alreadyRegistered {
			synced, err := g.bootstrapWorkspace(ctx, entry)
			if err != nil {
				return result, err
			}
			result.BootstrapNotes = synced
		}
	} else {
		result.BootstrapSkipped = true
	}

	g.mu.Lock()
	watcher := g.watcher
	g.mu.Unlock()
	if watcher != nil {
		if err := watcher.AddWorkspace(entry.RootPath); err != nil {
			return result, fmt.Errorf("watch workspace: %w", err)
		}
	}

	return result, nil
}

func (g *Guard) bootstrapWorkspace(ctx context.Context, ws state.WorkspaceEntry) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	g.mu.Lock()
	st := g.state
	session := g.session
	g.mu.Unlock()

	if err := g.client.Sync(session); err != nil {
		g.setLastSyncError(fmt.Sprintf("bw sync: %v", err))
		return 0, fmt.Errorf("sync vault: %w", err)
	}

	chain, err := g.client.LoadIncludeChain(session, ws.Folder)
	if err != nil {
		g.setLastSyncError(fmt.Sprintf("include chain: %v", err))
		return 0, err
	}
	chainNames := make([]string, len(chain))
	for i, f := range chain {
		chainNames[i] = f.Name
	}
	itemsByFolder := make(map[string][]types.Item, len(chain))
	for _, f := range chain {
		items, err := g.client.ListItems(session, f.ID)
		if err != nil {
			g.setLastSyncError(fmt.Sprintf("list items: %v", err))
			return 0, fmt.Errorf("list items for folder %q: %w", f.Name, err)
		}
		itemsByFolder[f.Name] = bw.FilterConfigNotes(items)
	}
	merged := note.MergeNoteItems(chainNames, itemsByFolder)
	notes := make([]types.Item, 0, len(merged.Items))
	sourceByID := make(map[string]string, len(merged.Items))
	for _, it := range merged.Items {
		notes = append(notes, it.Item)
		sourceByID[it.Item.ID] = it.Source
	}

	syncer := note.NewSyncer(st)
	synced, err := syncer.SyncFolderWithSources(notes, sourceByID, ws.TargetDir, ws.Folder)
	if err != nil {
		g.setLastSyncError(fmt.Sprintf("bootstrap sync: %v", err))
		return 0, err
	}
	if err := st.Save(); err != nil {
		g.setLastSyncError(fmt.Sprintf("save state: %v", err))
		return synced, fmt.Errorf("save state: %w", err)
	}
	g.clearLastSyncError()
	return synced, nil
}

func (g *Guard) onLocalChange(rootPath string) {
	_, _ = g.SyncWorkspace(context.Background(), rootPath)
}

func (g *Guard) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(g.pollEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			wasMissing := g.sessionWasMissing()
			hasSession := g.resolveSession(ctx)
			if !hasSession {
				continue
			}
			for _, ws := range g.state.ListWorkspaces() {
				_, _ = g.SyncWorkspace(ctx, ws.RootPath)
			}
			if wasMissing {
				g.clearLastSyncError()
			}
		}
	}
}

func (g *Guard) sessionWasMissing() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.sessionMissing
}

func (g *Guard) resolveSession(ctx context.Context) bool {
	g.mu.Lock()
	memory := g.session
	g.mu.Unlock()

	resolved, err := app.ResolveSession(ctx, memory)

	g.mu.Lock()
	defer g.mu.Unlock()

	if err != nil {
		g.session = ""
		g.sessionSource = app.SessionSourceNone
		g.sessionMissing = true
		g.lastSyncError = err.Error()
		return false
	}
	if !resolved.Valid {
		g.session = ""
		g.sessionSource = app.SessionSourceNone
		g.sessionMissing = true
		return false
	}

	g.session = resolved.Session
	g.sessionSource = resolved.Source
	g.sessionMissing = false
	return true
}

func (g *Guard) clearLastSyncError() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lastSyncError = ""
}

func (g *Guard) setLastSyncError(msg string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lastSyncError = msg
}

// SyncNow reconciles all registered workspaces, or one when rootPath is set.
func (g *Guard) SyncNow(ctx context.Context, rootPath ...string) ([]SyncSummary, error) {
	if !g.resolveSession(ctx) {
		return nil, nil
	}
	workspaces := g.state.ListWorkspaces()
	if len(rootPath) > 0 && rootPath[0] != "" {
		ws := g.state.FindWorkspace(rootPath[0])
		if ws == nil {
			return nil, fmt.Errorf("workspace not registered: %s", rootPath[0])
		}
		workspaces = []state.WorkspaceEntry{*ws}
	}
	var summaries []SyncSummary
	for _, ws := range workspaces {
		if err := ctx.Err(); err != nil {
			return summaries, err
		}
		summary, err := g.SyncWorkspace(ctx, ws.RootPath)
		if err != nil {
			return summaries, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

// SyncWorkspace reconciles notes for one registered workspace.
func (g *Guard) SyncWorkspace(ctx context.Context, rootPath string) (SyncSummary, error) {
	summary := SyncSummary{Workspace: rootPath}

	g.mu.Lock()
	st := g.state
	g.mu.Unlock()

	ws := st.FindWorkspace(rootPath)
	if ws == nil {
		return summary, fmt.Errorf("workspace not registered: %s", rootPath)
	}
	if err := ctx.Err(); err != nil {
		return summary, err
	}
	if !g.resolveSession(ctx) {
		return summary, nil
	}

	g.mu.Lock()
	session := g.session
	g.mu.Unlock()

	if err := g.client.Sync(session); err != nil {
		g.setLastSyncError(fmt.Sprintf("bw sync: %v", err))
		g.mu.Lock()
		g.session = ""
		g.mu.Unlock()
		_ = g.resolveSession(ctx)
		return summary, nil
	}

	folderID, err := g.client.GetFolderID(session, ws.Folder)
	if err != nil {
		g.setLastSyncError(fmt.Sprintf("folder lookup: %v", err))
		g.mu.Lock()
		g.session = ""
		g.mu.Unlock()
		_ = g.resolveSession(ctx)
		return summary, nil
	}
	items, err := g.client.ListItems(session, folderID)
	if err != nil {
		g.setLastSyncError(fmt.Sprintf("list items: %v", err))
		g.mu.Lock()
		g.session = ""
		g.mu.Unlock()
		_ = g.resolveSession(ctx)
		return summary, nil
	}
	notes := bw.FilterConfigNotes(items)
	remoteByID := make(map[string]types.Item, len(notes))
	for _, item := range notes {
		remoteByID[item.ID] = item
	}

	syncer := note.NewSyncer(st)
	if err := syncer.Preflight(notes, nil, ws.TargetDir, ws.Folder); err != nil {
		g.setLastSyncError(fmt.Sprintf("structural preflight aborted: %v", err))
		return summary, nil
	}

	pusher := BWNotePusher{Client: g.client, Session: session}
	tracked := st.FindSyncedNotes(ws.Folder, ws.TargetDir)
	var newConflicts []state.NoteEntry
	for _, entry := range tracked {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		remote, ok := remoteByID[entry.ItemID]
		if !ok {
			summary.Skipped++
			continue
		}
		localContent, localMod, err := readLocalNote(entry.FilePath)
		if err != nil {
			g.setLastSyncError(err.Error())
			return summary, nil
		}
		decision, err := DecideAction(entry, remote, localContent, localMod)
		if err != nil {
			g.setLastSyncError(err.Error())
			return summary, nil
		}
		if decision.Action == ActionNoop {
			summary.Skipped++
			continue
		}
		out, err := ReconcileNote(ReconcileInput{
			Entry:         entry,
			Remote:        remote,
			LocalContent:  localContent,
			LocalModTime:  localMod,
			WorkspaceRoot: ws.RootPath,
		}, decision, pusher)
		if err != nil {
			g.setLastSyncError(err.Error())
			return summary, nil
		}
		hadConflict := entry.Conflict != ""
		st.UpsertNote(out.UpdatedEntry)
		if out.UpdatedEntry.Conflict != "" {
			summary.Conflicts++
			if !hadConflict {
				newConflicts = append(newConflicts, out.UpdatedEntry)
			}
		} else {
			summary.Reconciled++
		}
	}
	g.notifyConflicts(newConflicts)
	if err := st.Save(); err != nil {
		g.setLastSyncError(fmt.Sprintf("save state: %v", err))
		return summary, nil
	}
	g.clearLastSyncError()
	return summary, nil
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

// PushNote uploads the local tracked note to Bitwarden.
func (g *Guard) PushNote(ctx context.Context, itemID, folder, targetDir string) error {
	return g.syncNote(ctx, itemID, folder, targetDir, ActionPushLocal)
}

// PullNote downloads the remote note into the local tracked file.
func (g *Guard) PullNote(ctx context.Context, itemID, folder, targetDir string) error {
	return g.syncNote(ctx, itemID, folder, targetDir, ActionPullRemote)
}

func (g *Guard) syncNote(ctx context.Context, itemID, folder, targetDir string, action NoteAction) error {
	if !g.resolveSession(ctx) {
		return fmt.Errorf("no valid Bitwarden session; %s", app.NeedsUnlockMessage())
	}

	g.mu.Lock()
	st := g.state
	session := g.session
	g.mu.Unlock()

	entry := st.FindNoteEntry(itemID, folder, targetDir)
	if entry == nil {
		return fmt.Errorf("note entry not found")
	}

	remote, err := g.client.GetItem(session, itemID)
	if err != nil {
		return err
	}

	localContent, localMod, err := readLocalNote(entry.FilePath)
	if err != nil {
		return err
	}

	ws := findWorkspaceForNote(st, folder, targetDir, entry.TargetDir)
	if ws == nil {
		return fmt.Errorf("workspace not found for note")
	}

	var pusher NotePusher
	if action == ActionPushLocal {
		pusher = BWNotePusher{Client: g.client, Session: session}
	}

	out, err := ReconcileNote(ReconcileInput{
		Entry:         *entry,
		Remote:        remote,
		LocalContent:  localContent,
		LocalModTime:  localMod,
		WorkspaceRoot: ws.RootPath,
	}, ReconcileDecision{Action: action}, pusher)
	if err != nil {
		return err
	}

	st.UpsertNote(out.UpdatedEntry)
	return st.Save()
}

// ResolveConflict clears a pending conflict by choosing local or remote copy.
func (g *Guard) ResolveConflict(ctx context.Context, itemID, folder, targetDir, choice string) error {
	if !g.resolveSession(ctx) {
		return fmt.Errorf("no valid Bitwarden session; %s", app.NeedsUnlockMessage())
	}

	g.mu.Lock()
	st := g.state
	session := g.session
	g.mu.Unlock()

	entry := st.FindNoteEntry(itemID, folder, targetDir)
	if entry == nil {
		return fmt.Errorf("note entry not found")
	}
	if entry.Conflict == "" {
		return fmt.Errorf("note has no active conflict")
	}
	ws := findWorkspaceForNote(st, folder, targetDir, entry.TargetDir)
	if ws == nil {
		return fmt.Errorf("workspace not found for note")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	remote, err := g.client.GetItem(session, itemID)
	if err != nil {
		return err
	}
	localContent, localMod, err := readLocalNote(entry.FilePath)
	if err != nil {
		return err
	}

	decision, err := parseConflictChoice(choice)
	if err != nil {
		return err
	}

	pusher := BWNotePusher{Client: g.client, Session: session}
	out, err := ReconcileNote(ReconcileInput{
		Entry:         *entry,
		Remote:        remote,
		LocalContent:  localContent,
		LocalModTime:  localMod,
		WorkspaceRoot: ws.RootPath,
	}, decision, pusher)
	if err != nil {
		return err
	}
	out.UpdatedEntry.Conflict = state.ConflictNone
	st.UpsertNote(out.UpdatedEntry)
	if err := st.Save(); err != nil {
		return err
	}
	_, err = DeleteConflictCopies(ws.RootPath, itemID)
	return err
}

func parseConflictChoice(choice string) (ReconcileDecision, error) {
	switch choice {
	case "local", "keep_local":
		return ReconcileDecision{Action: ActionPushLocal}, nil
	case "remote", "keep_remote":
		return ReconcileDecision{Action: ActionPullRemote}, nil
	default:
		return ReconcileDecision{}, fmt.Errorf("choice must be keep_local, keep_remote, local, or remote")
	}
}

func findWorkspaceForNote(st *state.State, folder, targetDir, entryTargetDir string) *state.WorkspaceEntry {
	if targetDir != "" {
		if ws, err := FindWorkspaceByFolderTarget(st, folder, targetDir); err == nil {
			return ws
		}
	}
	matchTarget := entryTargetDir
	if matchTarget == "" {
		matchTarget = targetDir
	}
	if matchTarget != "" {
		absTarget, err := filepath.Abs(matchTarget)
		if err == nil {
			for _, ws := range st.ListWorkspaces() {
				wsTarget := ws.TargetDir
				if wsTarget == "" {
					wsTarget = ws.RootPath
				}
				if ws.Folder == folder && wsTarget == absTarget {
					found := ws
					return &found
				}
			}
		}
	}
	for _, ws := range st.ListWorkspaces() {
		if ws.Folder == folder {
			found := ws
			return &found
		}
	}
	return nil
}
