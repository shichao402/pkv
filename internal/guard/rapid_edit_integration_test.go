//go:build integration

package guard

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shichao402/pkv/internal/bw"
	"github.com/shichao402/pkv/internal/state"
)

const rapidEditTestItemID = "5e1b42bf-265b-46f2-a24c-b46900aba25b"

// TestRapidLocalEditIntegration reproduces rapid local edits against live Bitwarden.
// Run: BW_SESSION=$(cat ~/.pkv/session) go test ./internal/guard -tags=integration -run TestRapidLocalEditIntegration -count=1 -v
func TestRapidLocalEditIntegration(t *testing.T) {
	session := strings.TrimSpace(os.Getenv("BW_SESSION"))
	if session == "" {
		t.Skip("BW_SESSION not set")
	}

	st, err := state.Load()
	if err != nil {
		t.Fatalf("state.Load() error = %v", err)
	}

	root := "/Users/firo/workspace/pkv"
	entry := st.FindNoteEntry(rapidEditTestItemID, "PKV", root)
	if entry == nil {
		t.Fatalf("test note entry not found in state")
	}

	client := bw.NewClient()

	// Align local file and BW to a known baseline.
	baseline := "baseline-" + time.Now().UTC().Format("150405")
	if err := os.WriteFile(entry.FilePath, []byte(baseline), 0o600); err != nil {
		t.Fatalf("WriteFile baseline error = %v", err)
	}
	pusher := BWNotePusher{Client: client, Session: session}
	if err := pusher.PushNoteContent(rapidEditTestItemID, baseline); err != nil {
		t.Fatalf("PushNoteContent baseline error = %v", err)
	}
	if err := client.Sync(session); err != nil {
		t.Fatalf("Sync baseline error = %v", err)
	}
	remote, err := client.GetItem(session, rapidEditTestItemID)
	if err != nil {
		t.Fatalf("GetItem after baseline error = %v", err)
	}
	rev, err := remote.RevisionTime()
	if err != nil {
		t.Fatalf("RevisionTime() error = %v", err)
	}
	now := time.Now().UTC()
	entry.ContentHash = hashContent(baseline)
	entry.LastSyncedAt = formatStateTime(now)
	entry.SyncedAt = entry.LastSyncedAt
	entry.LocalModifiedAt = formatStateTime(now)
	entry.RemoteRevisionAt = formatStateTime(rev)
	entry.Conflict = state.ConflictNone
	st.UpsertNote(*entry)
	if err := st.Save(); err != nil {
		t.Fatalf("Save baseline state error = %v", err)
	}

	conflictsBefore := countConflictFiles(root, rapidEditTestItemID)

	g := New(st, client, session)
	g.syncDebounce = 500 * time.Millisecond
	ctx := context.Background()

	values := []string{"", "1", "12", "123", "1234"}
	for _, v := range values {
		if err := os.WriteFile(entry.FilePath, []byte(v), 0o600); err != nil {
			t.Fatalf("write %q error = %v", v, err)
		}
		t.Logf("wrote %q at %s", v, time.Now().UTC().Format(time.RFC3339Nano))
		g.scheduleSync(root)
		time.Sleep(100 * time.Millisecond)
	}

	time.Sleep(800 * time.Millisecond)

	summary, err := g.SyncWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("SyncWorkspace() error = %v", err)
	}
	t.Logf("final sync: reconciled=%d conflicts=%d skipped=%d last_error=%q",
		summary.Reconciled, summary.Conflicts, summary.Skipped, g.Status().LastSyncError)
	if summary.Conflicts > 0 {
		t.Fatalf("conflict after rapid debounced edits")
	}

	conflictsAfter := countConflictFiles(root, rapidEditTestItemID)
	newConflicts := conflictsAfter - conflictsBefore

	remote, err = client.GetItem(session, rapidEditTestItemID)
	if err != nil {
		t.Fatalf("GetItem final error = %v", err)
	}

	t.Logf("remote notes = %q", remote.Notes)
	t.Logf("local file = %q", readFileString(t, entry.FilePath))
	t.Logf("new conflict files = %d (total %d)", newConflicts, conflictsAfter)

	if newConflicts > 0 {
		t.Fatalf("rapid local edit created %d conflict copies", newConflicts)
	}
	if remote.Notes != "1234" {
		t.Fatalf("remote notes = %q, want %q", remote.Notes, "1234")
	}
	if readFileString(t, entry.FilePath) != "1234" {
		t.Fatalf("local file out of sync with final edit")
	}
}

func countConflictFiles(root, itemID string) int {
	dir := filepath.Join(root, ".pkv", "conflicts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if strings.Contains(e.Name(), itemID) {
			n++
		}
	}
	return n
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(data)
}
