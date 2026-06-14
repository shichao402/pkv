package guard

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shichao402/pkv/internal/bw/types"
	"github.com/shichao402/pkv/internal/state"
)

func TestDecideActionOnlyRemote(t *testing.T) {
	last := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	entry := state.NoteEntry{
		ContentHash:      hashContent("local"),
		LastSyncedAt:     last.Format(time.RFC3339),
		RemoteRevisionAt: last.Format(time.RFC3339),
		LocalModifiedAt:  last.Format(time.RFC3339),
	}
	remote := types.Item{
		Notes:        "remote-new",
		RevisionDate: last.Add(time.Minute).Format(time.RFC3339),
	}

	decision, err := DecideAction(entry, remote, "local", last)
	if err != nil {
		t.Fatalf("DecideAction() error = %v", err)
	}
	if decision.Action != ActionPullRemote {
		t.Fatalf("action = %v, want ActionPullRemote", decision.Action)
	}
}

func TestDecideActionOnlyLocal(t *testing.T) {
	last := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	entry := state.NoteEntry{
		ContentHash:      hashContent("local-old"),
		LastSyncedAt:     last.Format(time.RFC3339),
		RemoteRevisionAt: last.Format(time.RFC3339),
		LocalModifiedAt:  last.Format(time.RFC3339),
	}
	remote := types.Item{
		Notes:        "local-old",
		RevisionDate: last.Format(time.RFC3339),
	}
	localMod := last.Add(2 * time.Minute)

	decision, err := DecideAction(entry, remote, "local-new", localMod)
	if err != nil {
		t.Fatalf("DecideAction() error = %v", err)
	}
	if decision.Action != ActionPushLocal {
		t.Fatalf("action = %v, want ActionPushLocal", decision.Action)
	}
}

func TestDecideActionBothSidesRemoteWins(t *testing.T) {
	last := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	entry := state.NoteEntry{
		ContentHash:      hashContent("baseline"),
		LastSyncedAt:     last.Format(time.RFC3339),
		RemoteRevisionAt: last.Format(time.RFC3339),
		LocalModifiedAt:  last.Format(time.RFC3339),
	}
	remote := types.Item{
		Notes:        "remote-new",
		RevisionDate: last.Add(3 * time.Minute).Format(time.RFC3339),
	}
	localMod := last.Add(2 * time.Minute)

	decision, err := DecideAction(entry, remote, "local-new", localMod)
	if err != nil {
		t.Fatalf("DecideAction() error = %v", err)
	}
	if decision.Action != ActionConflictRemoteWins {
		t.Fatalf("action = %v, want ActionConflictRemoteWins", decision.Action)
	}
}

func TestDecideActionBothSidesLocalWins(t *testing.T) {
	last := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	entry := state.NoteEntry{
		ContentHash:      hashContent("baseline"),
		LastSyncedAt:     last.Format(time.RFC3339),
		RemoteRevisionAt: last.Format(time.RFC3339),
		LocalModifiedAt:  last.Format(time.RFC3339),
	}
	remote := types.Item{
		Notes:        "remote-new",
		RevisionDate: last.Add(2 * time.Minute).Format(time.RFC3339),
	}
	localMod := last.Add(5 * time.Minute)

	decision, err := DecideAction(entry, remote, "local-new", localMod)
	if err != nil {
		t.Fatalf("DecideAction() error = %v", err)
	}
	if decision.Action != ActionConflictLocalWins {
		t.Fatalf("action = %v, want ActionConflictLocalWins", decision.Action)
	}
}

func TestDecideActionOwnPushRevisionNotRemoteDirty(t *testing.T) {
	lastSync := time.Date(2024, 1, 1, 12, 5, 0, 0, time.UTC)
	entry := state.NoteEntry{
		ContentHash:      hashContent("pushed"),
		LastSyncedAt:     lastSync.Format(time.RFC3339),
		RemoteRevisionAt: lastSync.Add(-time.Minute).Format(time.RFC3339),
		LocalModifiedAt:  lastSync.Add(-2 * time.Minute).Format(time.RFC3339),
	}
	remote := types.Item{
		Notes:        "pushed",
		RevisionDate: lastSync.Add(10 * time.Second).Format(time.RFC3339),
	}

	decision, err := DecideAction(entry, remote, "pushed", lastSync.Add(-2*time.Minute))
	if err != nil {
		t.Fatalf("DecideAction() error = %v", err)
	}
	if decision.Action != ActionNoop {
		t.Fatalf("action = %v, want ActionNoop when remote revision advanced after our push", decision.Action)
	}
}

func TestDecideActionRapidLocalEditAfterOwnPush(t *testing.T) {
	lastSync := time.Date(2024, 1, 1, 12, 5, 0, 0, time.UTC)
	entry := state.NoteEntry{
		ContentHash:      hashContent("pushed"),
		LastSyncedAt:     lastSync.Format(time.RFC3339),
		RemoteRevisionAt: lastSync.Add(-time.Minute).Format(time.RFC3339),
		LocalModifiedAt:  lastSync.Add(-2 * time.Minute).Format(time.RFC3339),
	}
	remote := types.Item{
		Notes:        "pushed",
		RevisionDate: lastSync.Add(10 * time.Second).Format(time.RFC3339),
	}
	localMod := lastSync.Add(2 * time.Second)

	decision, err := DecideAction(entry, remote, "1234", localMod)
	if err != nil {
		t.Fatalf("DecideAction() error = %v", err)
	}
	if decision.Action != ActionPushLocal {
		t.Fatalf("action = %v, want ActionPushLocal for rapid local-only edit", decision.Action)
	}
}

func TestDecideActionSameSecond(t *testing.T) {
	last := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	ts := last.Add(2 * time.Minute)
	entry := state.NoteEntry{
		ContentHash:      hashContent("baseline"),
		LastSyncedAt:     last.Format(time.RFC3339),
		RemoteRevisionAt: last.Format(time.RFC3339),
		LocalModifiedAt:  last.Format(time.RFC3339),
	}
	remote := types.Item{
		Notes:        "remote-new",
		RevisionDate: ts.Format(time.RFC3339),
	}

	decision, err := DecideAction(entry, remote, "local-new", ts.Add(500*time.Millisecond))
	if err != nil {
		t.Fatalf("DecideAction() error = %v", err)
	}
	if decision.Action != ActionConflictSameSecond {
		t.Fatalf("action = %v, want ActionConflictSameSecond", decision.Action)
	}
}


func TestReconcileNotePullRemote(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	last := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	entry := state.NoteEntry{
		ItemID:       "item-1",
		FileName:     "note.txt",
		FilePath:     path,
		ContentHash:  hashContent("old"),
		LastSyncedAt: last.Format(time.RFC3339),
	}
	remote := types.Item{Notes: "new-remote", RevisionDate: last.Add(time.Minute).Format(time.RFC3339)}

	out, err := ReconcileNote(ReconcileInput{
		Entry:         entry,
		Remote:        remote,
		LocalContent:  "old",
		WorkspaceRoot: dir,
	}, ReconcileDecision{Action: ActionPullRemote}, nil)
	if err != nil {
		t.Fatalf("ReconcileNote() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new-remote" {
		t.Fatalf("file content = %q, want %q", string(data), "new-remote")
	}
	if out.UpdatedEntry.Conflict != "" {
		t.Fatalf("conflict = %q, want empty", out.UpdatedEntry.Conflict)
	}
}

func TestReconcileNoteSameSecondPending(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	last := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	entry := state.NoteEntry{
		ItemID:       "item-1",
		FileName:     "note.txt",
		FilePath:     path,
		ContentHash:  hashContent("baseline"),
		LastSyncedAt: last.Format(time.RFC3339),
	}
	remote := types.Item{Notes: "remote", RevisionDate: last.Add(time.Minute).Format(time.RFC3339)}

	out, err := ReconcileNote(ReconcileInput{
		Entry:         entry,
		Remote:        remote,
		LocalContent:  "local",
		WorkspaceRoot: dir,
	}, ReconcileDecision{Action: ActionConflictSameSecond}, nil)
	if err != nil {
		t.Fatalf("ReconcileNote() error = %v", err)
	}
	if out.UpdatedEntry.Conflict != state.ConflictPending {
		t.Fatalf("conflict = %q, want pending", out.UpdatedEntry.Conflict)
	}
	if len(out.ConflictPaths) != 2 {
		t.Fatalf("conflict paths = %d, want 2", len(out.ConflictPaths))
	}
}
