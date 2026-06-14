package mcp

import (
	"context"
	"encoding/json"
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
	hooks.AddAfterInitialize(func(_ context.Context, _ any, _ *mcp.InitializeRequest, _ *mcp.InitializeResult) {
		go func() {
			_ = s.guard.RunInitPipeline(context.Background())
		}()
	})

	mcpServer := server.NewMCPServer(
		"pkv",
		version.Version,
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, true),
		server.WithInstructions(strings.TrimSpace(`
PKV MCP initializes on connect: session resolve and workspace auto-register from PKV_WORKSPACE_ROOT.

Notes are NOT auto-synced. Use pkv_download to pull from Bitwarden and pkv_upload to push local changes.
Read pkv://guard for init status, workspaces, and session.
When no valid session exists, pkv_download/pkv_upload open a local unlock page in your browser automatically.
Call pkv_register_workspace only when pkv://guard shows needs_config for workspace registration.
`)),
		server.WithHooks(hooks),
	)

	s.registerTools(mcpServer)
	s.registerResources(mcpServer)
	return mcpServer
}

func (s *Server) registerTools(mcpServer *server.MCPServer) {
	mcpServer.AddTool(mcp.NewTool("pkv_register_workspace",
		mcp.WithDescription("Register a workspace when auto-register failed (see pkv://guard needs_config)"),
		mcp.WithString("root_path", mcp.Required(), mcp.Description("Absolute workspace root path")),
		mcp.WithString("folder", mcp.Required(), mcp.Description("Bitwarden folder name")),
		mcp.WithString("target_dir", mcp.Description("Directory to sync notes into; defaults to root_path")),
	), s.handleRegisterWorkspace)
	mcpServer.AddTool(mcp.NewTool("pkv_download",
		mcp.WithDescription("Download notes from Bitwarden to local files (backs up existing local content)"),
		mcp.WithString("note", mcp.Description("Item ID or file name; omit or use 'all' for all tracked notes")),
		mcp.WithString("workspace_id", mcp.Description("Limit to one workspace root_path")),
	), s.handleDownload)
	mcpServer.AddTool(mcp.NewTool("pkv_upload",
		mcp.WithDescription("Upload local note files to Bitwarden (backs up remote content before overwrite)"),
		mcp.WithString("note", mcp.Description("Item ID or file name; omit or use 'all' for all tracked notes")),
		mcp.WithString("workspace_id", mcp.Description("Limit to one workspace root_path")),
	), s.handleUpload)
}

func (s *Server) registerResources(mcpServer *server.MCPServer) {
	mcpServer.AddResources(
		server.ServerResource{
			Resource: mcp.NewResource("pkv://guard", "Guard dashboard",
				mcp.WithResourceDescription("Init status, workspaces, session, and needs_config")),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				payload, err := s.guardPayload()
				if err != nil {
					return nil, err
				}
				return s.resourceJSON("pkv://guard", payload)
			},
		},
	)
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

func (s *Server) guardStatusPayload() map[string]any {
	guardStatus := s.guard.Status()
	payload := map[string]any{
		"ready":        guardStatus.Ready,
		"needs_config": guardStatus.NeedsConfig,
		"init":         s.guard.LastInitResult(),
		"session": map[string]any{
			"present": guardStatus.SessionPresent,
			"source":  guardStatus.SessionSource,
			"missing": guardStatus.SessionMissing,
		},
	}
	if guardStatus.SessionMissing {
		payload["needs_unlock"] = app.NeedsUnlockMessage()
		payload["web_unlock"] = "Call pkv_download or pkv_upload to unlock via browser"
	}
	return payload
}

func (s *Server) guardPayload() (map[string]any, error) {
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

	return map[string]any{
		"status":     s.guardStatusPayload(),
		"workspaces": views,
	}, nil
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
	text, _ := json.MarshalIndent(payload, "", "  ")
	return mcp.NewToolResultText(string(text)), nil
}

func (s *Server) handleDownload(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := s.ensureSessionForMCP(ctx); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	opts := guard.TransferOptions{
		Note:        req.GetString("note", ""),
		WorkspaceID: req.GetString("workspace_id", ""),
	}
	summary, err := s.guard.Download(ctx, opts)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	text, _ := json.MarshalIndent(summary, "", "  ")
	return mcp.NewToolResultText(string(text)), nil
}

func (s *Server) handleUpload(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := s.ensureSessionForMCP(ctx); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	opts := guard.TransferOptions{
		Note:        req.GetString("note", ""),
		WorkspaceID: req.GetString("workspace_id", ""),
	}
	summary, err := s.guard.Upload(ctx, opts)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	text, _ := json.MarshalIndent(summary, "", "  ")
	return mcp.NewToolResultText(string(text)), nil
}

// ServeStdio starts the MCP server over stdio transport.
func ServeStdio() error {
	lock, err := acquireStdioLock()
	if err != nil {
		return err
	}
	defer releaseStdioLock(lock)

	st, err := state.Load()
	if err != nil {
		return err
	}
	srv := NewServer(st)
	return server.ServeStdio(srv.MCPServer())
}

// Stop is a no-op retained for API compatibility.
func (s *Server) Stop() {}
