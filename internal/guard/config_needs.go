package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shichao402/pkv/internal/bw/types"
	"github.com/shichao402/pkv/internal/state"
)

func isUnexpandedTemplate(value string) bool {
	value = strings.TrimSpace(value)
	return strings.Contains(value, "${") && strings.Contains(value, "}")
}

// EffectiveWorkspaceRoot returns PKV_WORKSPACE_ROOT when set and expanded by the MCP host.
func EffectiveWorkspaceRoot() string {
	root := strings.TrimSpace(os.Getenv("PKV_WORKSPACE_ROOT"))
	if root == "" || isUnexpandedTemplate(root) {
		return ""
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	return absRoot
}

// DeriveConfigNeeds reports workspace configuration gaps for MCP self-service.
func DeriveConfigNeeds(envRoot string, st *state.State, folders []types.Folder) []ConfigNeed {
	root := strings.TrimSpace(envRoot)
	if root == "" {
		return []ConfigNeed{{
			Code:    "workspace_root_unset",
			Message: "PKV_WORKSPACE_ROOT is not set in the MCP server environment",
			Fix:     "Add PKV_WORKSPACE_ROOT=${workspaceFolder} to the MCP server env (e.g. .cursor/mcp.json)",
		}}
	}
	if isUnexpandedTemplate(root) {
		return []ConfigNeed{{
			Code:    "workspace_root_unset",
			Message: fmt.Sprintf("PKV_WORKSPACE_ROOT was not expanded by the MCP host (got %q)", root),
			Fix:     "Reload MCP so Cursor expands ${workspaceFolder}, or set PKV_WORKSPACE_ROOT to an absolute path",
		}}
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return []ConfigNeed{{
			Code:    "workspace_root_invalid",
			Message: fmt.Sprintf("PKV_WORKSPACE_ROOT is invalid: %v", err),
			Fix:     "Set PKV_WORKSPACE_ROOT to a valid workspace directory path",
		}}
	}

	if st == nil {
		return []ConfigNeed{{
			Code:    "workspace_unregistered",
			Message: fmt.Sprintf("Workspace %s is not registered", absRoot),
			Fix:     "Restart MCP or call pkv_register_workspace with root_path and folder",
		}}
	}

	ws := st.FindWorkspace(absRoot)
	if ws == nil {
		resolved := ResolveFolderCandidate(absRoot)
		return []ConfigNeed{{
			Code: "workspace_unregistered",
			Message: fmt.Sprintf(
				"Workspace %s is not registered (folder candidate %q from %s)",
				absRoot, resolved.Candidate, resolved.Source,
			),
			Fix: "Restart MCP to auto-register, or call pkv_register_workspace with root_path and folder",
		}}
	}

	if len(folders) == 0 {
		return nil
	}
	if folderExists(folders, ws.Folder) {
		return nil
	}
	resolved := ResolveFolderCandidate(absRoot)
	return []ConfigNeed{{
		Code: "folder_not_found",
		Message: fmt.Sprintf(
			"Bitwarden folder %q not found for workspace %s (inferred from %s)",
			ws.Folder, absRoot, resolved.Source,
		),
		Fix: fmt.Sprintf("Create folder %q in Bitwarden or set folder in .pkv/workspace.yaml", ws.Folder),
	}}
}
