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

func TestPollDoesNotRevertPendingLocalEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test")
	synced := "123"
	pending := "1234"
	if err := os.WriteFile(path, []byte(pending), 0o600); err != nil {
		t.Fatal(err)
	}

	lastSync := time.Date(2024, 1, 1, 12, 5, 0, 0, time.UTC)
	remoteRev := lastSync.Add(5 * time.Second)
	entry := state.NoteEntry{
		ItemID:           "item-test",
		FileName:         "test",
		FilePath:         path,
		ContentHash:      hashContent(synced),
		LastSyncedAt:     lastSync.Format(time.RFC3339),
		RemoteRevisionAt: remoteRev.Format(time.RFC3339),
		LocalModifiedAt:  lastSync.Format(time.RFC3339),
	}
	remote := types.Item{
		ID:           "item-test",
		Notes:        synced,
		RevisionDate: lastSync.Format(time.RFC3339),
	}
	remoteNew := types.Item{
		ID:           "item-test",
		Notes:        "456",
		RevisionDate: remoteRev.Add(time.Minute).Format(time.RFC3339),
	}

	// Without pending edit, remote-only change would pull.
	cleanDecision, err := DecideAction(entry, remoteNew, synced, lastSync)
	if err != nil {
		t.Fatalf("DecideAction(clean) error = %v", err)
	}
	if cleanDecision.Action != ActionPullRemote {
		t.Fatalf("clean action = %v, want ActionPullRemote", cleanDecision.Action)
	}

	pendingDecision, err := DecideAction(entry, remote, pending, lastSync.Add(time.Second))
	if err != nil {
		t.Fatalf("DecideAction(pending) error = %v", err)
	}
	if pendingDecision.Action == ActionPullRemote {
		t.Fatalf("pending action = %v, must not pull while local edit is pending", pendingDecision.Action)
	}
	if pendingDecision.Action != ActionPushLocal {
		t.Fatalf("pending action = %v, want ActionPushLocal", pendingDecision.Action)
	}

	pollDecision := guardPollPendingEdit(true, entry, pending, pendingDecision)
	if pollDecision.Action != ActionNoop {
		t.Fatalf("poll guard action = %v, want ActionNoop during poll", pollDecision.Action)
	}

	_, err = ReconcileNote(ReconcileInput{
		Entry:         entry,
		Remote:        remote,
		LocalContent:  pending,
		LocalModTime:  lastSync.Add(time.Second),
		WorkspaceRoot: dir,
	}, pollDecision, nil)
	if err != nil {
		t.Fatalf("ReconcileNote() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != pending {
		t.Fatalf("local file = %q, want %q after poll noop", string(data), pending)
	}
}

func TestDebouncePushAfterRapidEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test")
	baseline := "123"
	if err := os.WriteFile(path, []byte(baseline), 0o600); err != nil {
		t.Fatal(err)
	}

	lastSync := time.Date(2024, 1, 1, 12, 5, 0, 0, time.UTC)
	entry := state.NoteEntry{
		ItemID:           "item-test",
		FileName:         "test",
		FilePath:         path,
		ContentHash:      hashContent(baseline),
		LastSyncedAt:     lastSync.Format(time.RFC3339),
		RemoteRevisionAt: lastSync.Format(time.RFC3339),
		LocalModifiedAt:  lastSync.Format(time.RFC3339),
	}
	remote := types.Item{
		ID:           "item-test",
		Notes:        baseline,
		RevisionDate: lastSync.Format(time.RFC3339),
	}

	var pushContents []string
	pusher := &recordingPusher{fn: func(_ string, content string) error {
		pushContents = append(pushContents, content)
		return nil
	}}

	// Rapid edits: poll cycles must not push or pull mid-edit.
	for _, v := range []string{"1234", "12345", "123456"} {
		if err := os.WriteFile(path, []byte(v), 0o600); err != nil {
			t.Fatal(err)
		}
		decision, err := DecideAction(entry, remote, v, time.Now())
		if err != nil {
			t.Fatalf("DecideAction(%q) error = %v", v, err)
		}
		guarded := guardPollPendingEdit(true, entry, v, decision)
		if guarded.Action != ActionNoop {
			t.Fatalf("poll guard for %q = %v, want ActionNoop", v, guarded.Action)
		}
	}

	// After debounce: one push with final content only.
	final := "123456"
	if err := os.WriteFile(path, []byte(final), 0o600); err != nil {
		t.Fatal(err)
	}
	decision, err := DecideAction(entry, remote, final, time.Now())
	if err != nil {
		t.Fatalf("DecideAction(final) error = %v", err)
	}
	guarded := guardPollPendingEdit(false, entry, final, decision)
	if guarded.Action != ActionPushLocal {
		t.Fatalf("debounced action = %v, want ActionPushLocal", guarded.Action)
	}
	_, err = ReconcileNote(ReconcileInput{
		Entry:         entry,
		Remote:        remote,
		LocalContent:  final,
		WorkspaceRoot: dir,
	}, guarded, pusher)
	if err != nil {
		t.Fatalf("ReconcileNote() error = %v", err)
	}
	if len(pushContents) != 1 {
		t.Fatalf("push count = %d, want 1 after debounce", len(pushContents))
	}
	if pushContents[0] != final {
		t.Fatalf("pushed content = %q, want %q", pushContents[0], final)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != final {
		t.Fatalf("local file = %q, want %q", string(data), final)
	}
}

func TestDecideActionPendingEditNeverPulls(t *testing.T) {
	lastSync := time.Date(2024, 1, 1, 12, 5, 0, 0, time.UTC)
	entry := state.NoteEntry{
		ContentHash:      hashContent("123"),
		LastSyncedAt:     lastSync.Format(time.RFC3339),
		RemoteRevisionAt: lastSync.Format(time.RFC3339),
		LocalModifiedAt:  lastSync.Format(time.RFC3339),
	}
	remote := types.Item{
		Notes:        "remote-new",
		RevisionDate: lastSync.Add(time.Minute).Format(time.RFC3339),
	}

	decision, err := DecideAction(entry, remote, "local-new", lastSync.Add(time.Second))
	if err != nil {
		t.Fatalf("DecideAction() error = %v", err)
	}
	if decision.Action == ActionPullRemote {
		t.Fatalf("action = %v, pending local edit must never pull", decision.Action)
	}
}

func TestWorkspaceDirtyWhileDebouncePending(t *testing.T) {
	st := &state.State{}
	root := t.TempDir()
	if _, _, err := RegisterWorkspace(st, root, "test-folder", ""); err != nil {
		t.Fatal(err)
	}

	g := New(st, nil, "test-session")
	if g.isWorkspaceDirty(root) {
		t.Fatal("workspace should not be dirty initially")
	}

	g.scheduleSync(root)
	if !g.isWorkspaceDirty(root) {
		t.Fatal("workspace should be dirty after scheduleSync")
	}

	g.syncScheduleMu.Lock()
	g.dirtyRoots = make(map[string]struct{})
	g.syncScheduleMu.Unlock()
	if g.isWorkspaceDirty(root) {
		t.Fatal("workspace should not be dirty after clearing dirtyRoots")
	}
}

type recordingPusher struct {
	fn func(itemID, content string) error
}

func (p *recordingPusher) PushNoteContent(itemID, content string) error {
	return p.fn(itemID, content)
}
