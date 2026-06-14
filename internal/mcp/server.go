package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/shichao402/pkv/internal/app"
	"github.com/shichao402/pkv/internal/bw"
	"github.com/shichao402/pkv/internal/guard"
	"github.com/shichao402/pkv/internal/state"
	"github.com/shichao402/pkv/internal/version"
)

type Server struct {
	guard *guard.Guard
	state *state.State
}

func NewServer(st *state.State) *Server {
	if st == nil {
		var err error
		st, err = state.Load()
		if err != nil {
			st = &state.State{}
		}
	}
	return &Server{
		state: st,
		guard: guard.New(st, bw.NewClient(), strings.TrimSpace(os.Getenv("BW_SESSION"))),
	}
}

func (s *Server) MCPServer() *server.MCPServer {
	hooks := &server.Hooks{}
	hooks.AddAfterInitialize(func(ctx context.Context, id any, message *mcp.InitializeRequest, result *mcp.InitializeResult) {
		_ = s.guard.Start(context.Background())
		go func() {
			bg := context.Background()
			_, _ = s.guard.AutoRegisterFromEnv(bg)
			_, _ = s.guard.SyncNow(bg)
		}()
	})

	mcpServer := server.NewMCPServer(
		"pkv",
		version.Version,
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, true),
		server.WithHooks(hooks),
	)

	s.registerTools(mcpServer)
	s.registerResources(mcpServer)
	s.guard.SetConflictNotifier(func(notes []state.NoteEntry) {
		for _, note := range notes {
			mcpServer.SendNotificationToAllClients(string(mcp.MethodNotificationMessage), map[string]any{
				"level":  mcp.LoggingLevelWarning,
				"logger": "pkv-guard",
				"data": map[string]any{
					"event":      "conflict",
					"item_id":    note.ItemID,
					"file_name":  note.FileName,
					"folder":     note.Folder,
					"target_dir": note.TargetDir,
					"file_path":  note.FilePath,
				},
			})
		}
	})
	return mcpServer
}

func (s *Server) registerTools(mcpServer *server.MCPServer) {
	mcpServer.AddTool(mcp.NewTool("pkv_status",
		mcp.WithDescription("Report PKV guard sync status"),
	), s.handleStatus)
	mcpServer.AddTool(mcp.NewTool("pkv_unlock",
		mcp.WithDescription("Validate BW_SESSION or instruct user to export session"),
		mcp.WithString("session", mcp.Description("Optional Bitwarden session to persist and use")),
	), s.handleUnlock)
	mcpServer.AddTool(mcp.NewTool("pkv_register_workspace",
		mcp.WithDescription("Register a workspace for guard note sync"),
		mcp.WithString("root_path", mcp.Required(), mcp.Description("Absolute workspace root path")),
		mcp.WithString("folder", mcp.Required(), mcp.Description("Bitwarden folder name")),
		mcp.WithString("target_dir", mcp.Description("Directory to sync notes into; defaults to root_path")),
	), s.handleRegisterWorkspace)
	mcpServer.AddTool(mcp.NewTool("pkv_list_workspaces",
		mcp.WithDescription("List registered guard sync workspaces"),
	), s.handleListWorkspaces)
	mcpServer.AddTool(mcp.NewTool("pkv_unregister_workspace",
		mcp.WithDescription("Remove a registered workspace"),
		mcp.WithString("workspace_id", mcp.Description("Workspace root path (same as root_path at registration)")),
		mcp.WithString("root_path", mcp.Description("Alias for workspace_id")),
	), s.handleUnregisterWorkspace)
	mcpServer.AddTool(mcp.NewTool("pkv_sync_now",
		mcp.WithDescription("Run guard reconciliation for registered workspaces"),
		mcp.WithString("workspace_id", mcp.Description("Sync only this workspace (root_path)")),
		mcp.WithString("folder", mcp.Description("Bitwarden folder; requires target_dir")),
		mcp.WithString("target_dir", mcp.Description("Sync target directory; requires folder")),
	), s.handleSyncNow)
	mcpServer.AddTool(mcp.NewTool("pkv_list_conflicts",
		mcp.WithDescription("List note entries with active conflicts"),
	), s.handleListConflicts)
	mcpServer.AddTool(mcp.NewTool("pkv_resolve_conflict",
		mcp.WithDescription("Resolve a pending note conflict"),
		mcp.WithString("item_id", mcp.Required()),
		mcp.WithString("folder", mcp.Required()),
		mcp.WithString("target_dir", mcp.Description("Synced target directory")),
		mcp.WithString("choice", mcp.Required(), mcp.Description("keep_local, keep_remote, local, or remote")),
	), s.handleResolveConflict)
	mcpServer.AddTool(mcp.NewTool("pkv_show_conflict",
		mcp.WithDescription("Show canonical path and conflict copies for a pending note"),
		mcp.WithString("item_id", mcp.Required()),
	), s.handleShowConflict)
	mcpServer.AddTool(mcp.NewTool("pkv_push_note",
		mcp.WithDescription("Push a tracked local note to Bitwarden"),
		mcp.WithString("item_id", mcp.Required()),
		mcp.WithString("folder", mcp.Required()),
		mcp.WithString("target_dir", mcp.Description("Synced target directory")),
	), s.handlePushNote)
	mcpServer.AddTool(mcp.NewTool("pkv_pull_note",
		mcp.WithDescription("Pull a remote note into the tracked local file"),
		mcp.WithString("item_id", mcp.Required()),
		mcp.WithString("folder", mcp.Required()),
		mcp.WithString("target_dir", mcp.Description("Synced target directory")),
	), s.handlePullNote)
}

func (s *Server) registerResources(mcpServer *server.MCPServer) {
	mcpServer.AddResources(
		server.ServerResource{
			Resource: mcp.NewResource("pkv://status", "Guard sync status"),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				return s.resourceJSON("pkv://status", s.statusPayload())
			},
		},
		server.ServerResource{
			Resource: mcp.NewResource("pkv://workspaces", "Registered guard workspaces"),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				return s.resourceJSON("pkv://workspaces", s.workspacesPayload())
			},
		},
		server.ServerResource{
			Resource: mcp.NewResource("pkv://conflicts", "Pending note conflicts"),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				payload, err := s.conflictsPayload()
				if err != nil {
					return nil, err
				}
				return s.resourceJSON("pkv://conflicts", payload)
			},
		},
	)
	mcpServer.AddResourceTemplate(
		mcp.NewResourceTemplate("pkv://conflicts/{item_id}", "Conflict detail for one note"),
		func(_ context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			itemID := conflictItemIDFromRequest(request)
			if itemID == "" {
				return nil, fmt.Errorf("item_id is required")
			}
			detail, err := guard.ShowConflict(s.state, itemID)
			if err != nil {
				return nil, err
			}
			return s.resourceJSON(request.Params.URI, detail)
		},
	)
}

func conflictItemIDFromRequest(request mcp.ReadResourceRequest) string {
	if args := request.Params.Arguments; args != nil {
		if raw, ok := args["item_id"]; ok {
			switch v := raw.(type) {
			case []string:
				if len(v) > 0 {
					return strings.TrimSpace(v[0])
				}
			case string:
				return strings.TrimSpace(v)
			}
		}
	}
	uri := strings.TrimSpace(request.Params.URI)
	const prefix = "pkv://conflicts/"
	if strings.HasPrefix(uri, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(uri, prefix))
	}
	return ""
}

func (s *Server) resourceJSON(uri string, payload any) ([]mcp.ResourceContents, error) {
	text, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, err
	}
	return []mcp.ResourceContents{mcp.TextResourceContents{
		URI:      uri,
		MIMEType: "application/json",
		Text:     string(text),
	}}, nil
}

func (s *Server) statusPayload() map[string]any {
	workspaces := guard.ListRegisteredWorkspaces(s.state)
	conflicts := s.state.ListConflictNotes()
	guardStatus := s.guard.Status()
	return map[string]any{
		"workspaces":       len(workspaces),
		"conflicts":        len(conflicts),
		"session_present":  guardStatus.SessionPresent,
		"session_source":   guardStatus.SessionSource,
		"session_missing":  guardStatus.SessionMissing,
		"watch_running":    guardStatus.WatchRunning,
		"last_sync_error":  guardStatus.LastSyncError,
		"needs_unlock":     guardStatus.NeedsUnlock,
	}
}

func (s *Server) workspacesPayload() map[string]any {
	workspaces := guard.ListRegisteredWorkspaces(s.state)
	type workspaceView struct {
		WorkspaceID  string `json:"workspace_id"`
		RootPath     string `json:"root_path"`
		Folder       string `json:"folder"`
		TargetDir    string `json:"target_dir"`
		RegisteredAt string `json:"registered_at"`
	}
	views := make([]workspaceView, 0, len(workspaces))
	for _, ws := range workspaces {
		views = append(views, workspaceView{
			WorkspaceID:  ws.RootPath,
			RootPath:     ws.RootPath,
			Folder:       ws.Folder,
			TargetDir:    ws.TargetDir,
			RegisteredAt: ws.RegisteredAt,
		})
	}
	return map[string]any{"workspaces": views}
}

func (s *Server) conflictsPayload() (map[string]any, error) {
	conflicts := s.state.ListConflictNotes()
	filesByWorkspace := make(map[string][]string)
	for _, ws := range s.state.ListWorkspaces() {
		paths, err := guard.ListConflictFiles(ws.RootPath)
		if err != nil {
			return nil, err
		}
		if len(paths) > 0 {
			filesByWorkspace[ws.RootPath] = paths
		}
	}
	return map[string]any{
		"notes":          conflicts,
		"conflict_files": filesByWorkspace,
	}, nil
}

func (s *Server) handleStatus(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	payload := s.statusPayload()
	text, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		workspaces := guard.ListRegisteredWorkspaces(s.state)
		conflicts := s.state.ListConflictNotes()
		return mcp.NewToolResultText(fmt.Sprintf("status: workspaces=%d conflicts=%d", len(workspaces), len(conflicts))), nil
	}
	return mcp.NewToolResultText(string(text)), nil
}

func (s *Server) handleUnlock(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	wasMissing := s.guard.Status().SessionMissing
	injected := strings.TrimSpace(req.GetString("session", ""))
	if injected != "" {
		if err := app.ValidateBitwardenSession(ctx, injected); err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("injected session invalid: %v\n%s", err, app.NeedsUnlockMessage())), nil
		}
		if err := app.WriteSession(injected); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		s.guard.ApplyResolvedSession(app.ResolvedSession{
			Session: injected,
			Source:  app.SessionSourceFile,
			Valid:   true,
		})
		if wasMissing {
			summaries, syncErr := s.guard.SyncNow(ctx)
			if syncErr != nil {
				return mcp.NewToolResultText(fmt.Sprintf("Session injected and saved but sync failed: %v", syncErr)), nil
			}
			text, _ := json.MarshalIndent(map[string]any{
				"message":   "Session injected, saved, and sync completed.",
				"summaries": summaries,
			}, "", "  ")
			return mcp.NewToolResultText(string(text)), nil
		}
		return mcp.NewToolResultText("Session injected and saved."), nil
	}

	resolved, err := app.ResolveSession(ctx, s.guard.Session())
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("session resolve failed: %v\n%s", err, app.NeedsUnlockMessage())), nil
	}
	if !resolved.Valid {
		return mcp.NewToolResultText(fmt.Sprintf("No valid Bitwarden session.\n%s", app.NeedsUnlockMessage())), nil
	}
	s.guard.ApplyResolvedSession(resolved)
	if wasMissing {
		summaries, syncErr := s.guard.SyncNow(ctx)
		if syncErr != nil {
			return mcp.NewToolResultText(fmt.Sprintf("Session restored from %s but sync failed: %v", resolved.Source, syncErr)), nil
		}
		text, _ := json.MarshalIndent(map[string]any{
			"message":   fmt.Sprintf("Session restored from %s; sync completed.", resolved.Source),
			"summaries": summaries,
		}, "", "  ")
		return mcp.NewToolResultText(string(text)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Session valid (source=%s).", resolved.Source)), nil
}

func (s *Server) handleRegisterWorkspace(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rootPath, err := req.RequireString("root_path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	folder, err := req.RequireString("folder")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	targetDir := req.GetString("target_dir", "")
	result, err := s.guard.RegisterWorkspace(ctx, rootPath, folder, targetDir)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := s.state.Save(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	payload := map[string]any{"workspace": result.Entry}
	if result.AlreadyRegistered {
		payload["already_registered"] = true
	}
	if result.FolderValidated {
		payload["folder_validated"] = true
	}
	if result.BootstrapSkipped {
		payload["bootstrap_skipped"] = true
	}
	if result.BootstrapNotes > 0 {
		payload["bootstrap_notes"] = result.BootstrapNotes
	}
	if line, ok := guard.GitIgnoreSuggestion(result.Entry.RootPath); ok {
		payload["gitignore_suggestion"] = map[string]string{
			"line":    line,
			"message": "git repository detected; add this line to .gitignore to avoid committing conflict copies",
		}
	}
	text, _ := json.MarshalIndent(payload, "", "  ")
	return mcp.NewToolResultText(string(text)), nil
}

func (s *Server) handleListWorkspaces(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	text, _ := json.MarshalIndent(s.workspacesPayload(), "", "  ")
	return mcp.NewToolResultText(string(text)), nil
}

func (s *Server) handleUnregisterWorkspace(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rootPath := strings.TrimSpace(req.GetString("workspace_id", ""))
	if rootPath == "" {
		rootPath = strings.TrimSpace(req.GetString("root_path", ""))
	}
	if rootPath == "" {
		return mcp.NewToolResultError("workspace_id or root_path is required"), nil
	}
	if err := guard.UnregisterWorkspace(s.state, rootPath); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := s.state.Save(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("workspace unregistered: %s", rootPath)), nil
}

func (s *Server) handleSyncNow(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	workspaceID := strings.TrimSpace(req.GetString("workspace_id", ""))
	folder := strings.TrimSpace(req.GetString("folder", ""))
	targetDir := strings.TrimSpace(req.GetString("target_dir", ""))

	ws, err := guard.ResolveSyncWorkspace(s.state, workspaceID, folder, targetDir)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var summaries []guard.SyncSummary
	if ws != nil {
		summaries, err = s.guard.SyncNow(ctx, ws.RootPath)
	} else {
		summaries, err = s.guard.SyncNow(ctx)
	}
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if s.guard.Status().SessionMissing {
		return mcp.NewToolResultText(fmt.Sprintf("No valid Bitwarden session.\n%s", app.NeedsUnlockMessage())), nil
	}
	text, _ := json.MarshalIndent(summaries, "", "  ")
	return mcp.NewToolResultText(string(text)), nil
}

func (s *Server) handleListConflicts(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	payload, err := s.conflictsPayload()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	text, _ := json.MarshalIndent(payload, "", "  ")
	return mcp.NewToolResultText(string(text)), nil
}

func (s *Server) handleResolveConflict(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	itemID, err := req.RequireString("item_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	folder, err := req.RequireString("folder")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	choice, err := req.RequireString("choice")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	targetDir := req.GetString("target_dir", "")
	if s.guard.Status().SessionMissing {
		return mcp.NewToolResultError(fmt.Sprintf("no valid Bitwarden session; %s", app.NeedsUnlockMessage())), nil
	}
	if err := s.guard.ResolveConflict(ctx, itemID, folder, targetDir, choice); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText("conflict resolved"), nil
}

func (s *Server) handleShowConflict(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	itemID, err := req.RequireString("item_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	detail, err := guard.ShowConflict(s.state, itemID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	text, _ := json.MarshalIndent(detail, "", "  ")
	return mcp.NewToolResultText(string(text)), nil
}

func (s *Server) handlePushNote(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	itemID, err := req.RequireString("item_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	folder, err := req.RequireString("folder")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	targetDir := req.GetString("target_dir", "")
	if err := s.guard.PushNote(ctx, itemID, folder, targetDir); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText("note pushed"), nil
}

func (s *Server) handlePullNote(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	itemID, err := req.RequireString("item_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	folder, err := req.RequireString("folder")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	targetDir := req.GetString("target_dir", "")
	if err := s.guard.PullNote(ctx, itemID, folder, targetDir); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText("note pulled"), nil
}

// ServeStdio starts the MCP server over stdio transport.
func ServeStdio() error {
	st, err := state.Load()
	if err != nil {
		return err
	}
	srv := NewServer(st)
	defer srv.Stop()
	return server.ServeStdio(srv.MCPServer())
}

// Stop shuts down guard sync resources.
func (s *Server) Stop() {
	if s.guard != nil {
		s.guard.Stop()
	}
}
