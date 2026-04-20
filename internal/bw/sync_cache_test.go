package bw

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSyncReusesRecentSyncForSameSession(t *testing.T) {
	resetSyncTestState(t, time.Date(2026, 4, 20, 13, 0, 0, 0, time.UTC))
	logPath := filepath.Join(t.TempDir(), "bw.log")

	client := NewClient()
	client.execCommand = newTestBWExecCommand(t, "sync_ok", logPath)

	if err := client.Sync("session-a"); err != nil {
		t.Fatalf("first Sync() error = %v", err)
	}
	if err := client.Sync("session-a"); err != nil {
		t.Fatalf("second Sync() error = %v", err)
	}

	assertBWCalls(t, logPath,
		"bw --nointeraction --session session-a sync|env=",
	)
}

func TestSyncDoesNotReuseAcrossSessions(t *testing.T) {
	resetSyncTestState(t, time.Date(2026, 4, 20, 13, 0, 0, 0, time.UTC))
	logPath := filepath.Join(t.TempDir(), "bw.log")

	client := NewClient()
	client.execCommand = newTestBWExecCommand(t, "sync_ok", logPath)

	if err := client.Sync("session-a"); err != nil {
		t.Fatalf("Sync(session-a) error = %v", err)
	}
	if err := client.Sync("session-b"); err != nil {
		t.Fatalf("Sync(session-b) error = %v", err)
	}

	assertBWCalls(t, logPath,
		"bw --nointeraction --session session-a sync|env=",
		"bw --nointeraction --session session-b sync|env=",
	)
}

func TestSyncRefreshesAfterReuseWindowExpires(t *testing.T) {
	baseNow := time.Date(2026, 4, 20, 13, 0, 0, 0, time.UTC)
	resetSyncTestState(t, baseNow)
	logPath := filepath.Join(t.TempDir(), "bw.log")

	client := NewClient()
	client.execCommand = newTestBWExecCommand(t, "sync_ok", logPath)

	if err := client.Sync("session-a"); err != nil {
		t.Fatalf("first Sync() error = %v", err)
	}

	syncNow = func() time.Time { return baseNow.Add(syncReuseWindow + time.Millisecond) }

	if err := client.Sync("session-a"); err != nil {
		t.Fatalf("second Sync() after reuse window error = %v", err)
	}

	assertBWCalls(t, logPath,
		"bw --nointeraction --session session-a sync|env=",
		"bw --nointeraction --session session-a sync|env=",
	)
}

func TestCreateItemInvalidatesRecentSync(t *testing.T) {
	resetSyncTestState(t, time.Date(2026, 4, 20, 13, 0, 0, 0, time.UTC))
	logPath := filepath.Join(t.TempDir(), "bw.log")

	client := NewClient()
	client.execCommand = newTestBWExecCommand(t, "sync_create_ok", logPath)

	if err := client.Sync("session-a"); err != nil {
		t.Fatalf("initial Sync() error = %v", err)
	}
	if _, err := client.CreateItem("session-a", []byte(`{"type":2,"name":"demo"}`)); err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}
	if err := client.Sync("session-a"); err != nil {
		t.Fatalf("Sync() after create error = %v", err)
	}

	calls := readTestBWCalls(t, logPath)
	if len(calls) != 3 {
		t.Fatalf("bw calls = %#v, want 3 calls", calls)
	}
	if calls[0] != "bw --nointeraction --session session-a sync|env=" {
		t.Fatalf("first bw call = %q", calls[0])
	}
	if !strings.HasPrefix(calls[1], "bw --nointeraction --session session-a create item ") {
		t.Fatalf("second bw call = %q, want create item", calls[1])
	}
	if calls[2] != "bw --nointeraction --session session-a sync|env=" {
		t.Fatalf("third bw call = %q", calls[2])
	}
}

func TestEditItemInvalidatesRecentSync(t *testing.T) {
	resetSyncTestState(t, time.Date(2026, 4, 20, 13, 0, 0, 0, time.UTC))
	logPath := filepath.Join(t.TempDir(), "bw.log")

	client := NewClient()
	client.execCommand = newTestBWExecCommand(t, "sync_edit_ok", logPath)

	if err := client.Sync("session-a"); err != nil {
		t.Fatalf("initial Sync() error = %v", err)
	}
	if err := client.EditItem("session-a", "item-1", []byte(`{"id":"item-1","type":2}`)); err != nil {
		t.Fatalf("EditItem() error = %v", err)
	}
	if err := client.Sync("session-a"); err != nil {
		t.Fatalf("Sync() after edit error = %v", err)
	}

	calls := readTestBWCalls(t, logPath)
	if len(calls) != 3 {
		t.Fatalf("bw calls = %#v, want 3 calls", calls)
	}
	if calls[0] != "bw --nointeraction --session session-a sync|env=" {
		t.Fatalf("first bw call = %q", calls[0])
	}
	if !strings.HasPrefix(calls[1], "bw --nointeraction --session session-a edit item item-1 ") {
		t.Fatalf("second bw call = %q, want edit item", calls[1])
	}
	if calls[2] != "bw --nointeraction --session session-a sync|env=" {
		t.Fatalf("third bw call = %q", calls[2])
	}
}

func TestDeleteItemInvalidatesRecentSync(t *testing.T) {
	resetSyncTestState(t, time.Date(2026, 4, 20, 13, 0, 0, 0, time.UTC))
	logPath := filepath.Join(t.TempDir(), "bw.log")

	client := NewClient()
	client.execCommand = newTestBWExecCommand(t, "sync_delete_ok", logPath)

	if err := client.Sync("session-a"); err != nil {
		t.Fatalf("initial Sync() error = %v", err)
	}
	if err := client.DeleteItem("session-a", "item-1"); err != nil {
		t.Fatalf("DeleteItem() error = %v", err)
	}
	if err := client.Sync("session-a"); err != nil {
		t.Fatalf("Sync() after delete error = %v", err)
	}

	assertBWCalls(t, logPath,
		"bw --nointeraction --session session-a sync|env=",
		"bw --nointeraction --session session-a delete item item-1|env=",
		"bw --nointeraction --session session-a sync|env=",
	)
}

func resetSyncTestState(t *testing.T, now time.Time) {
	t.Helper()
	invalidateSyncCache()
	oldSyncNow := syncNow
	syncNow = func() time.Time { return now }
	t.Cleanup(func() {
		invalidateSyncCache()
		syncNow = oldSyncNow
	})
}

func assertBWCalls(t *testing.T, logPath string, want ...string) {
	t.Helper()
	if got := readTestBWCalls(t, logPath); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("bw calls = %#v, want %#v", got, want)
	}
}
