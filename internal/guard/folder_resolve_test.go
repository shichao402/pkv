package guard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shichao402/pkv/internal/bw/types"
)

func TestResolveFolderCandidateWorkspaceYAML(t *testing.T) {
	root := t.TempDir()
	pkvDir := filepath.Join(root, ".pkv")
	if err := os.MkdirAll(pkvDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkvDir, "workspace.yaml"), []byte("folder: MY_PROJECT\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := ResolveFolderCandidate(root)
	if got.Candidate != "MY_PROJECT" || got.Source != "workspace_yaml" {
		t.Fatalf("ResolveFolderCandidate() = %+v, want MY_PROJECT from workspace_yaml", got)
	}
}

func TestResolveFolderCandidateDecConfig(t *testing.T) {
	root := t.TempDir()
	decDir := filepath.Join(root, ".dec")
	if err := os.MkdirAll(decDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(decDir, "config.yaml"), []byte("pkv_folder: PKV\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := ResolveFolderCandidate(root)
	if got.Candidate != "PKV" || got.Source != "dec_config" {
		t.Fatalf("ResolveFolderCandidate() = %+v, want PKV from dec_config", got)
	}
}

func TestResolveFolderCandidateBasename(t *testing.T) {
	root := filepath.Join(t.TempDir(), "myapp")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	got := ResolveFolderCandidate(root)
	if got.Candidate != "myapp" || got.Source != "basename" {
		t.Fatalf("ResolveFolderCandidate() = %+v, want myapp from basename", got)
	}
}

func TestMatchFolderNameCaseInsensitive(t *testing.T) {
	folders := []types.Folder{{Name: "PKV", ID: "1"}}
	name, ok := MatchFolderName(folders, "pkv")
	if !ok || name != "PKV" {
		t.Fatalf("MatchFolderName() = %q, %v; want PKV, true", name, ok)
	}
}
