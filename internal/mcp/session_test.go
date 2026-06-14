package mcp

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/shichao402/pkv/internal/app"
	"github.com/shichao402/pkv/internal/guard"
	"github.com/shichao402/pkv/internal/state"
	"github.com/shichao402/pkv/internal/unlock"
)

func TestInitDoesNotStartWebUnlock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BW_SESSION", "")

	var browserOpens atomic.Int32
	restore := unlock.SetOpenBrowserForTest(func(string) error {
		browserOpens.Add(1)
		return nil
	})
	defer restore()

	st := &state.State{}
	srv := &Server{
		state: st,
		guard: guard.New(st, nil, ""),
	}

	_ = srv.guard.RunInitPipeline(context.Background())

	if browserOpens.Load() != 0 {
		t.Fatalf("browser opens during init = %d, want 0", browserOpens.Load())
	}
	if !srv.guard.Status().SessionMissing {
		t.Fatal("expected session missing after init without file")
	}
}

func TestEnsureSessionUsesFileWithoutWebUnlock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BW_SESSION", "")

	restoreValidator := app.SetSessionValidatorForTest(func(_ context.Context, session string) error {
		if session == "file-session" {
			return nil
		}
		return context.Canceled
	})
	defer restoreValidator()

	var browserOpens atomic.Int32
	restoreBrowser := unlock.SetOpenBrowserForTest(func(string) error {
		browserOpens.Add(1)
		return nil
	})
	defer restoreBrowser()

	if err := app.WriteSession("file-session"); err != nil {
		t.Fatal(err)
	}

	st := &state.State{}
	srv := &Server{
		state: st,
		guard: guard.New(st, nil, ""),
	}

	if err := srv.ensureSessionForMCP(context.Background()); err != nil {
		t.Fatalf("ensureSessionForMCP() error = %v", err)
	}
	if browserOpens.Load() != 0 {
		t.Fatalf("browser opens = %d, want 0 when session file valid", browserOpens.Load())
	}
	if !srv.guard.Status().SessionPresent {
		t.Fatal("SessionPresent = false after loading file session")
	}
}

func TestEnsureSessionStartsWebUnlockWhenMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BW_SESSION", "")

	started := make(chan string, 1)
	restoreBrowser := unlock.SetOpenBrowserForTest(func(pageURL string) error {
		started <- pageURL
		return nil
	})
	defer restoreBrowser()

	st := &state.State{}
	srv := &Server{
		state: st,
		guard: guard.New(st, nil, ""),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- srv.ensureSessionForMCP(ctx)
	}()

	select {
	case url := <-started:
		if url == "" {
			t.Fatal("expected unlock URL")
		}
		cancel()
	case err := <-done:
		if err == nil {
			t.Fatal("ensureSessionForMCP succeeded without unlock")
		}
		t.Fatalf("ensureSessionForMCP returned before opening browser: %v", err)
	}
}
