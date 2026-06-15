package guard

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/shichao402/pkv/internal/app"
	"github.com/shichao402/pkv/internal/bw"
	"github.com/shichao402/pkv/internal/bw/types"
	"github.com/shichao402/pkv/internal/note"
	"github.com/shichao402/pkv/internal/state"
)

type ConfigNeed struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
}

type InitStep struct {
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type InitResult struct {
	Status string     `json:"status,omitempty"`
	OK     bool       `json:"ok"`
	Steps  []InitStep `json:"steps,omitempty"`
}

type Status struct {
	Ready          bool         `json:"ready"`
	NeedsConfig    []ConfigNeed `json:"needs_config,omitempty"`
	SessionPresent bool         `json:"session_present"`
	SessionSource  string       `json:"session_source"`
	SessionMissing bool         `json:"session_missing"`
	NeedsUnlock    string       `json:"needs_unlock,omitempty"`
}

type Guard struct {
	state   *state.State
	client  *bw.Client
	session string

	mu             sync.Mutex
	sessionSource  app.SessionSource
	sessionMissing bool
	lastInitResult InitResult
	initRunning    bool
}

type RegisterWorkspaceResult struct {
	Entry             state.WorkspaceEntry `json:"entry"`
	AlreadyRegistered bool                 `json:"already_registered,omitempty"`
	FolderValidated   bool                 `json:"folder_validated,omitempty"`
	BootstrapSkipped  bool                 `json:"bootstrap_skipped,omitempty"`
	BootstrapNotes    int                  `json:"bootstrap_notes,omitempty"`
}

func New(st *state.State, client *bw.Client, session string) *Guard {
	if client == nil {
		client = bw.NewClient()
	}
	return &Guard{
		state:          st,
		client:         client,
		session:        session,
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

func (g *Guard) Status() Status {
	g.mu.Lock()
	session := g.session
	sessionMissing := g.sessionMissing
	sessionSource := g.sessionSource
	st := g.state
	client := g.client
	g.mu.Unlock()

	status := Status{
		SessionPresent: session != "" && !sessionMissing,
		SessionSource:  string(sessionSource),
		SessionMissing: sessionMissing,
	}
	if status.SessionMissing {
		status.NeedsUnlock = app.NeedsUnlockMessage()
	}
	if status.SessionSource == "" {
		status.SessionSource = string(app.SessionSourceNone)
	}

	var folders []types.Folder
	if status.SessionPresent && client != nil {
		if listed, err := client.ListFolders(session); err == nil {
			folders = listed
		}
	}
	status.NeedsConfig = DeriveConfigNeeds(EffectiveWorkspaceRoot(), st, folders)
	status.Ready = len(status.NeedsConfig) == 0
	return status
}

func (g *Guard) LastInitResult() InitResult {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.initSnapshotLocked()
}

func (g *Guard) initSnapshotLocked() InitResult {
	if g.initRunning {
		return InitResult{Status: "running", OK: false}
	}
	if len(g.lastInitResult.Steps) == 0 {
		return InitResult{Status: "pending", OK: false}
	}
	out := g.lastInitResult
	out.Status = "done"
	return out
}

func (g *Guard) setLastInitResult(result InitResult) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lastInitResult = result
}

// RunInitPipeline resolves session, auto-registers workspace from env, and saves state.
func (g *Guard) RunInitPipeline(ctx context.Context) InitResult {
	g.mu.Lock()
	if g.initRunning {
		snapshot := g.initSnapshotLocked()
		g.mu.Unlock()
		return snapshot
	}
	g.initRunning = true
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		g.initRunning = false
		g.mu.Unlock()
	}()

	result := InitResult{OK: true}
	record := func(name string, ok bool, err error) {
		step := InitStep{Name: name, OK: ok}
		if err != nil {
			step.Error = err.Error()
			result.OK = false
		}
		result.Steps = append(result.Steps, step)
	}

	sessionOK := g.resolveSession(ctx)
	record("resolve_session", sessionOK, nil)

	_, regErr := g.AutoRegisterFromEnv(ctx)
	record("auto_register", regErr == nil, regErr)

	g.mu.Lock()
	st := g.state
	g.mu.Unlock()
	saveErr := st.Save()
	record("save", saveErr == nil, saveErr)

	g.setLastInitResult(result)
	return result
}

// RegisterWorkspace validates, persists, and optionally bootstraps a workspace.
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
			return result, fmt.Errorf("list Bitwarden folders: %w", err)
		}
		if !folderExists(folders, folder) {
			return result, fmt.Errorf("bitwarden folder not found: %s", folder)
		}
		canonical, _ := MatchFolderName(folders, folder)
		if canonical != "" && entry.Folder != canonical {
			entry.Folder = canonical
			st.RegisterWorkspace(entry)
			result.Entry = entry
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
		return 0, fmt.Errorf("sync vault: %w", err)
	}

	notes, sourceByID, err := g.loadMergedNotes(session, ws.Folder)
	if err != nil {
		return 0, err
	}

	syncer := note.NewSyncer(st)
	synced, err := syncer.SyncFolderWithSources(notes, sourceByID, ws.TargetDir, ws.Folder)
	if err != nil {
		return 0, err
	}
	if err := st.Save(); err != nil {
		return synced, fmt.Errorf("save state: %w", err)
	}
	return synced, nil
}

func (g *Guard) loadMergedNotes(session, folder string) ([]types.Item, map[string]string, error) {
	chain, err := g.client.LoadIncludeChain(session, folder)
	if err != nil {
		return nil, nil, fmt.Errorf("include chain: %w", err)
	}
	chainNames := make([]string, len(chain))
	for i, f := range chain {
		chainNames[i] = f.Name
	}
	itemsByFolder := make(map[string][]types.Item, len(chain))
	for _, f := range chain {
		items, err := g.client.ListItems(session, f.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("list items for folder %q: %w", f.Name, err)
		}
		itemsByFolder[f.Name] = bw.FilterConfigNotes(items)
	}
	merged := note.MergeNoteItems(chainNames, itemsByFolder)
	notes := make([]types.Item, 0, len(merged.Items))
	sourceByID := make(map[string]string, len(merged.Items))
	for i := range merged.Items {
		it := merged.Items[i]
		notes = append(notes, it.Item)
		sourceByID[it.Item.ID] = it.Source
	}
	return notes, sourceByID, nil
}

// AutoRegisterFromEnv registers PKV_WORKSPACE_ROOT when not already registered.
func (g *Guard) AutoRegisterFromEnv(ctx context.Context) (RegisterWorkspaceResult, error) {
	absRoot := EffectiveWorkspaceRoot()
	if absRoot == "" {
		return RegisterWorkspaceResult{}, nil
	}

	g.mu.Lock()
	st := g.state
	g.mu.Unlock()

	if existing := st.FindWorkspace(absRoot); existing != nil {
		return RegisterWorkspaceResult{
			Entry:             *existing,
			AlreadyRegistered: true,
		}, nil
	}

	resolved := ResolveFolderCandidate(absRoot)
	result, err := g.RegisterWorkspace(ctx, absRoot, resolved.Candidate, "")
	if err != nil {
		return result, err
	}
	if !result.AlreadyRegistered {
		if err := st.Save(); err != nil {
			return result, fmt.Errorf("save state: %w", err)
		}
	}
	return result, nil
}

// ResolveSessionFromSources reloads session from memory, env, or ~/.pkv/session.
func (g *Guard) ResolveSessionFromSources(ctx context.Context) bool {
	return g.resolveSession(ctx)
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
