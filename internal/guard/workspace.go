package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shichao402/pkv/internal/bw/types"
	"github.com/shichao402/pkv/internal/state"
)

// RegisterWorkspace validates local path/folder input and persists a workspace registration.
// When the same root path and folder are registered again, the existing entry is returned.
func RegisterWorkspace(st *state.State, rootPath, folder, targetDir string) (state.WorkspaceEntry, bool, error) {
	if st == nil {
		return state.WorkspaceEntry{}, false, fmt.Errorf("state is nil")
	}
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return state.WorkspaceEntry{}, false, fmt.Errorf("resolve workspace path: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return state.WorkspaceEntry{}, false, fmt.Errorf("workspace path: %w", err)
	}
	if !info.IsDir() {
		return state.WorkspaceEntry{}, false, fmt.Errorf("workspace path is not a directory: %s", absRoot)
	}
	if folder == "" {
		return state.WorkspaceEntry{}, false, fmt.Errorf("folder is required")
	}
	absTarget := absRoot
	if targetDir != "" {
		absTarget, err = filepath.Abs(targetDir)
		if err != nil {
			return state.WorkspaceEntry{}, false, fmt.Errorf("resolve target dir: %w", err)
		}
		targetInfo, err := os.Stat(absTarget)
		if err != nil {
			return state.WorkspaceEntry{}, false, fmt.Errorf("target dir: %w", err)
		}
		if !targetInfo.IsDir() {
			return state.WorkspaceEntry{}, false, fmt.Errorf("target dir is not a directory: %s", absTarget)
		}
	}

	for _, existing := range st.ListWorkspaces() {
		if existing.RootPath != absRoot {
			continue
		}
		if strings.EqualFold(existing.Folder, folder) {
			if targetDir != "" && existing.TargetDir != absTarget {
				existing.TargetDir = absTarget
				st.RegisterWorkspace(existing)
			}
			return existing, true, nil
		}
		return state.WorkspaceEntry{}, false, fmt.Errorf("workspace already registered with different folder: %s (have %q, want %q)", absRoot, existing.Folder, folder)
	}

	entry := state.WorkspaceEntry{
		RootPath:  absRoot,
		Folder:    folder,
		TargetDir: absTarget,
	}
	st.RegisterWorkspace(entry)
	return entry, false, nil
}

func folderExists(folders []types.Folder, name string) bool {
	_, ok := MatchFolderName(folders, name)
	return ok
}

// FindWorkspaceByID returns a workspace by root path (workspace_id).
func FindWorkspaceByID(st *state.State, workspaceID string) (*state.WorkspaceEntry, error) {
	if st == nil {
		return nil, fmt.Errorf("state is nil")
	}
	absID, err := filepath.Abs(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace id: %w", err)
	}
	ws := st.FindWorkspace(absID)
	if ws == nil {
		return nil, fmt.Errorf("workspace not registered: %s", absID)
	}
	return ws, nil
}

// FindWorkspaceByFolderTarget returns a workspace by folder and target directory.
func FindWorkspaceByFolderTarget(st *state.State, folder, targetDir string) (*state.WorkspaceEntry, error) {
	if st == nil {
		return nil, fmt.Errorf("state is nil")
	}
	if folder == "" {
		return nil, fmt.Errorf("folder is required")
	}
	if targetDir == "" {
		return nil, fmt.Errorf("target_dir is required")
	}
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, fmt.Errorf("resolve target dir: %w", err)
	}
	for _, ws := range st.ListWorkspaces() {
		wsTarget := ws.TargetDir
		if wsTarget == "" {
			wsTarget = ws.RootPath
		}
		if ws.Folder == folder && wsTarget == absTarget {
			found := ws
			return &found, nil
		}
	}
	return nil, fmt.Errorf("workspace not registered for folder=%s target_dir=%s", folder, absTarget)
}

// ResolveSyncWorkspace picks one workspace for targeted sync.
func ResolveSyncWorkspace(st *state.State, workspaceID, folder, targetDir string) (*state.WorkspaceEntry, error) {
	switch {
	case workspaceID != "":
		return FindWorkspaceByID(st, workspaceID)
	case folder != "" && targetDir != "":
		return FindWorkspaceByFolderTarget(st, folder, targetDir)
	case folder != "" || targetDir != "":
		return nil, fmt.Errorf("folder and target_dir must be provided together")
	default:
		return nil, nil
	}
}

// GitIgnoreSuggestion returns a gitignore line when root is a git repo.
func GitIgnoreSuggestion(rootPath string) (string, bool) {
	if _, err := os.Stat(filepath.Join(rootPath, ".git")); err != nil {
		return "", false
	}
	return ".pkv/conflicts/", true
}

// UnregisterWorkspace removes a workspace registration from state.
func UnregisterWorkspace(st *state.State, rootPath string) error {
	if st == nil {
		return fmt.Errorf("state is nil")
	}
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return fmt.Errorf("resolve workspace path: %w", err)
	}
	if !st.UnregisterWorkspace(absRoot) {
		return fmt.Errorf("workspace not registered: %s", absRoot)
	}
	return nil
}

// ListRegisteredWorkspaces returns all workspace entries from state.
func ListRegisteredWorkspaces(st *state.State) []state.WorkspaceEntry {
	if st == nil {
		return nil
	}
	return st.ListWorkspaces()
}
