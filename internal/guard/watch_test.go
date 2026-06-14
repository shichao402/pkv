package guard

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/shichao402/pkv/internal/state"
)

func TestWatcherAddWorkspaceRecursive(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "notes")
	pkvDir := filepath.Join(root, ".pkv")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pkvDir, 0o755); err != nil {
		t.Fatal(err)
	}

	var (
		mu      sync.Mutex
		changed []string
	)
	w := NewWatcher(func(rootPath string) {
		mu.Lock()
		changed = append(changed, rootPath)
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.AddWorkspace(root); err != nil {
		t.Fatalf("AddWorkspace() error = %v", err)
	}
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer w.Stop()

	notePath := filepath.Join(sub, "config.md")
	if err := os.WriteFile(notePath, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	pkvNote := filepath.Join(pkvDir, "secret.md")
	if err := os.WriteFile(pkvNote, []byte("ignore"), 0o644); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		n := len(changed)
		mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(changed) == 0 {
		t.Fatal("expected handler to fire for nested note change")
	}
	if changed[0] != root {
		t.Fatalf("handler root = %q, want %q", changed[0], root)
	}
}

func TestGuardAddWorkspaceWhileRunning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BW_SESSION", "")

	st := &state.State{}
	g := New(st, nil, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := g.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer g.Stop()

	root := t.TempDir()
	result, err := g.RegisterWorkspace(context.Background(), root, "test-folder", "")
	if err != nil {
		t.Fatalf("RegisterWorkspace() error = %v", err)
	}
	if !result.BootstrapSkipped {
		t.Fatal("BootstrapSkipped = false, want true without session")
	}
	if !g.Status().WatchRunning {
		t.Fatal("WatchRunning = false, want true")
	}
}
