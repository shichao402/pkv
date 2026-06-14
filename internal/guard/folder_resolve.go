package guard

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/shichao402/pkv/internal/bw/types"
)

// FolderResolveResult describes how a Bitwarden folder name was inferred.
type FolderResolveResult struct {
	Candidate string
	Source    string // workspace_yaml | dec_config | basename
}

// ResolveFolderCandidate picks a Bitwarden folder name for a workspace root.
// Priority: .pkv/workspace.yaml → .dec/config.yaml pkv_folder → directory basename.
func ResolveFolderCandidate(workspaceRoot string) FolderResolveResult {
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		absRoot = workspaceRoot
	}

	if folder, ok := readYAMLString(filepath.Join(absRoot, ".pkv", "workspace.yaml"), "folder"); ok && folder != "" {
		return FolderResolveResult{Candidate: folder, Source: "workspace_yaml"}
	}
	if folder, ok := readYAMLString(filepath.Join(absRoot, ".dec", "config.yaml"), "pkv_folder"); ok && folder != "" {
		return FolderResolveResult{Candidate: folder, Source: "dec_config"}
	}
	return FolderResolveResult{
		Candidate: filepath.Base(absRoot),
		Source:    "basename",
	}
}

func readYAMLString(path, key string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()

	prefix := key + ":"
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		value = strings.Trim(value, `"'`)
		return value, value != ""
	}
	return "", false
}

// MatchFolderName returns the canonical Bitwarden folder name (case-sensitive BW spelling).
func MatchFolderName(folders []types.Folder, name string) (string, bool) {
	for _, f := range folders {
		if f.Name == name {
			return f.Name, true
		}
	}
	for _, f := range folders {
		if strings.EqualFold(f.Name, name) {
			return f.Name, true
		}
	}
	return "", false
}
