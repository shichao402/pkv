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

	result := g.RunInitPipeline(context.Background())
	if len(result.Steps) != 3 {
		t.Fatalf("Steps len = %d, want 3: %+v", len(result.Steps), result.Steps)
	}
	wantNames := []string{"resolve_session", "auto_register", "save"}
	for i, name := range wantNames {
		if result.Steps[i].Name != name {
			t.Fatalf("Steps[%d].Name = %q, want %q", i, result.Steps[i].Name, name)
		}
	}
	if !result.Steps[2].OK {
		t.Fatalf("save step failed: %+v", result.Steps[2])
	}

	stored := g.LastInitResult()
	if stored.Status != "done" {
		t.Fatalf("LastInitResult status = %q, want done", stored.Status)
	}
	if len(stored.Steps) != 3 {
		t.Fatalf("LastInitResult steps = %d, want 3", len(stored.Steps))
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
