package guard

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shichao402/pkv/internal/app"
	"github.com/shichao402/pkv/internal/bw/types"
	"github.com/shichao402/pkv/internal/state"
)

func TestScheduleSyncDebouncesRapidChanges(t *testing.T) {
	t.Setenv("PKV_SYNC_DEBOUNCE", "150ms")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BW_SESSION", "test-session")

	restore := app.SetSessionValidatorForTest(func(_ context.Context, session string) error {
		if session == "test-session" {
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

	var cycles atomic.Int32
	g := New(st, nil, "test-session")
	g.syncDebounce = 150 * time.Millisecond
	g.syncCycleHook = func() { cycles.Add(1) }

	for i := 0; i < 5; i++ {
		g.scheduleSync(root)
	}
	time.Sleep(400 * time.Millisecond)

	if n := cycles.Load(); n != 1 {
		t.Fatalf("sync cycles = %d, want 1 coalesced cycle", n)
	}
}

func TestRapidWritesCoalesceToOnePush(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	baseline := "baseline"
	if err := os.WriteFile(path, []byte(baseline), 0o600); err != nil {
		t.Fatal(err)
	}

	last := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	entry := state.NoteEntry{
		ItemID:       "item-1",
		FileName:     "note.txt",
		FilePath:     path,
		ContentHash:  hashContent(baseline),
		LastSyncedAt: last.Format(time.RFC3339),
	}
	remote := types.Item{
		ID:           "item-1",
		Notes:        baseline,
		RevisionDate: last.Format(time.RFC3339),
	}

	var pushContents []string
	pusher := &recordingPusher{fn: func(_ string, content string) error {
		pushContents = append(pushContents, content)
		return nil
	}}

	// Simulating per-write sync (no debounce) would push every intermediate value.
	perWritePushes := 0
	for _, v := range []string{"1", "12", "123", "1234"} {
		decision, err := DecideAction(entry, remote, v, time.Now())
		if err != nil {
			t.Fatalf("DecideAction(%q) error = %v", v, err)
		}
		if decision.Action == ActionPushLocal {
			perWritePushes++
		}
	}
	if perWritePushes != 4 {
		t.Fatalf("per-write pushes = %d, want 4 intermediate pushes without coalesce", perWritePushes)
	}

	// After debounce: one reconcile with final content only.
	final := "1234"
	decision, err := DecideAction(entry, remote, final, time.Now())
	if err != nil {
		t.Fatalf("DecideAction(final) error = %v", err)
	}
	if decision.Action != ActionPushLocal {
		t.Fatalf("action = %v, want ActionPushLocal", decision.Action)
	}
	_, err = ReconcileNote(ReconcileInput{
		Entry:         entry,
		Remote:        remote,
		LocalContent:  final,
		WorkspaceRoot: dir,
	}, decision, pusher)
	if err != nil {
		t.Fatalf("ReconcileNote() error = %v", err)
	}
	if len(pushContents) != 1 {
		t.Fatalf("push count = %d, want 1", len(pushContents))
	}
	if pushContents[0] != final {
		t.Fatalf("pushed content = %q, want %q", pushContents[0], final)
	}
}

func TestOnLocalChangeSchedulesNotImmediateSync(t *testing.T) {
	t.Setenv("PKV_SYNC_DEBOUNCE", "200ms")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BW_SESSION", "")

	restore := app.SetSessionValidatorForTest(func(_ context.Context, session string) error {
		if session == "test-session" {
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

	var cycles atomic.Int32
	g := New(st, nil, "")
	g.syncDebounce = 200 * time.Millisecond
	g.syncCycleHook = func() { cycles.Add(1) }

	g.onLocalChange(root)
	if cycles.Load() != 0 {
		t.Fatalf("sync ran immediately, cycles = %d", cycles.Load())
	}

	time.Sleep(350 * time.Millisecond)
	if cycles.Load() != 0 {
		t.Fatalf("sync ran without session, cycles = %d", cycles.Load())
	}

	g.SetSession("test-session")
	g.onLocalChange(root)
	time.Sleep(350 * time.Millisecond)
	if n := cycles.Load(); n != 1 {
		t.Fatalf("sync cycles = %d, want 1 after debounce with session", n)
	}
}

type recordingPusher struct {
	fn func(itemID, content string) error
}

func (p *recordingPusher) PushNoteContent(itemID, content string) error {
	return p.fn(itemID, content)
}
