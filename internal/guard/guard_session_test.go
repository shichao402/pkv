package guard

import (
	"context"
	"fmt"
	"testing"

	"github.com/shichao402/pkv/internal/app"
	"github.com/shichao402/pkv/internal/state"
)

func TestGuardStartWithoutSessionDoesNotPanic(t *testing.T) {
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

	status := g.Status()
	if !status.SessionMissing {
		t.Fatalf("SessionMissing = false, want true")
	}
	if status.NeedsUnlock == "" {
		t.Fatal("NeedsUnlock is empty, want guidance message")
	}
}

func TestGuardSyncWithoutSessionSkipsGracefully(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BW_SESSION", "")

	st := &state.State{}
	g := New(st, nil, "")
	root := t.TempDir()
	if _, _, err := RegisterWorkspace(st, root, "test-folder", ""); err != nil {
		t.Fatal(err)
	}

	summary, err := g.SyncWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("SyncWorkspace() error = %v, want nil", err)
	}
	if summary.Workspace == "" {
		t.Fatal("summary workspace is empty")
	}

	status := g.Status()
	if !status.SessionMissing {
		t.Fatalf("SessionMissing = false, want true")
	}
}

func TestGuardSyncNowRecoversSessionFromFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BW_SESSION", "")

	restore := app.SetSessionValidatorForTest(func(_ context.Context, session string) error {
		if session == "recovered-session" {
			return nil
		}
		return fmt.Errorf("invalid session")
	})
	defer restore()

	st := &state.State{}
	root := t.TempDir()
	if _, _, err := RegisterWorkspace(st, root, "test-folder", ""); err != nil {
		t.Fatal(err)
	}
	g := New(st, nil, "")

	if status := g.Status(); !status.SessionMissing {
		t.Fatalf("initial SessionMissing = false, want true")
	}

	if err := app.WriteSession("recovered-session"); err != nil {
		t.Fatal(err)
	}

	if _, err := g.SyncNow(context.Background()); err != nil {
		t.Fatalf("SyncNow() error = %v", err)
	}

	status := g.Status()
	if !status.SessionPresent {
		t.Fatalf("SessionPresent = false after recovery, status = %+v", status)
	}
	if status.SessionSource != string(app.SessionSourceFile) {
		t.Fatalf("SessionSource = %q, want file", status.SessionSource)
	}
}
