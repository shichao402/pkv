package guard

import (
	"context"
	"fmt"
	"testing"

	"github.com/shichao402/pkv/internal/app"
	"github.com/shichao402/pkv/internal/state"
)

func TestGuardStatusWithoutSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BW_SESSION", "")

	st := &state.State{}
	g := New(st, nil, "")

	status := g.Status()
	if !status.SessionMissing {
		t.Fatalf("SessionMissing = false, want true")
	}
	if status.NeedsUnlock == "" {
		t.Fatal("NeedsUnlock is empty, want guidance message")
	}
}

func TestGuardResolveSessionFromFile(t *testing.T) {
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
	g := New(st, nil, "")

	if status := g.Status(); !status.SessionMissing {
		t.Fatalf("initial SessionMissing = false, want true")
	}

	if err := app.WriteSession("recovered-session"); err != nil {
		t.Fatal(err)
	}

	if !g.resolveSession(context.Background()) {
		t.Fatal("resolveSession() = false, want true")
	}

	status := g.Status()
	if !status.SessionPresent {
		t.Fatalf("SessionPresent = false after recovery, status = %+v", status)
	}
	if status.SessionSource != string(app.SessionSourceFile) {
		t.Fatalf("SessionSource = %q, want file", status.SessionSource)
	}
}
