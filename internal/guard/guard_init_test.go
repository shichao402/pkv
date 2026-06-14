package guard

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shichao402/pkv/internal/state"
)

func TestRunInitPipelineRecordsSteps(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BW_SESSION", "")

	root := t.TempDir()
	t.Setenv("PKV_WORKSPACE_ROOT", root)

	st, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := New(st, nil, "")
	defer g.Stop()

	result := g.RunInitPipeline(context.Background())
	if len(result.Steps) != 5 {
		t.Fatalf("Steps len = %d, want 5: %+v", len(result.Steps), result.Steps)
	}
	wantNames := []string{"start", "resolve_session", "auto_register", "sync", "save"}
	for i, name := range wantNames {
		if result.Steps[i].Name != name {
			t.Fatalf("Steps[%d].Name = %q, want %q", i, result.Steps[i].Name, name)
		}
	}
	if !result.Steps[0].OK {
		t.Fatalf("start step failed: %+v", result.Steps[0])
	}
	if !result.Steps[3].OK {
		t.Fatalf("sync step failed: %+v", result.Steps[3])
	}
	if !result.Steps[4].OK {
		t.Fatalf("save step failed: %+v", result.Steps[4])
	}

	stored := g.LastInitResult()
	if stored.Status != "done" {
		t.Fatalf("LastInitResult status = %q, want done", stored.Status)
	}
	if len(stored.Steps) != 5 {
		t.Fatalf("LastInitResult steps = %d, want 5", len(stored.Steps))
	}
}

func TestAutoRegisterFromEnvPersistsWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BW_SESSION", "")

	root := t.TempDir()
	t.Setenv("PKV_WORKSPACE_ROOT", root)

	st, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := New(st, nil, "")

	if _, err := g.AutoRegisterFromEnv(context.Background()); err != nil {
		t.Fatalf("AutoRegisterFromEnv() error = %v", err)
	}

	path := filepath.Join(home, ".pkv", "state.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state file missing after auto-register: %v", err)
	}
	if st.FindWorkspace(root) == nil {
		t.Fatal("workspace not in memory after auto-register")
	}
}
