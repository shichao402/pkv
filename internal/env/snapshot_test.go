package env

import (
	"os"
	"path/filepath"
	"testing"
)

// setupSnapshotDir creates a temp dir and overrides HOME so snapshot
// functions write to an isolated location. Returns cleanup function.
func setupSnapshotDir(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	return tmpDir
}

func TestSaveAndLoadSnapshot(t *testing.T) {
	setupSnapshotDir(t)

	snap := Snapshot{
		ItemID: "item1",
		Name:   "github1",
		Vars:   map[string]string{"TOKEN": "aaa", "API_URL": "xxx"},
	}

	if err := SaveSnapshot(snap); err != nil {
		t.Fatalf("SaveSnapshot() error = %v", err)
	}

	loaded, err := LoadSnapshot("item1")
	if err != nil {
		t.Fatalf("LoadSnapshot() error = %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadSnapshot() returned nil")
	}
	if loaded.ItemID != "item1" {
		t.Errorf("ItemID = %q, want %q", loaded.ItemID, "item1")
	}
	if loaded.Name != "github1" {
		t.Errorf("Name = %q, want %q", loaded.Name, "github1")
	}
	if loaded.Vars["TOKEN"] != "aaa" {
		t.Errorf("Vars[TOKEN] = %q, want %q", loaded.Vars["TOKEN"], "aaa")
	}
	if loaded.Vars["API_URL"] != "xxx" {
		t.Errorf("Vars[API_URL] = %q, want %q", loaded.Vars["API_URL"], "xxx")
	}
}

func TestLoadSnapshot_NotExist(t *testing.T) {
	setupSnapshotDir(t)

	loaded, err := LoadSnapshot("nonexistent")
	if err != nil {
		t.Fatalf("LoadSnapshot() error = %v", err)
	}
	if loaded != nil {
		t.Errorf("LoadSnapshot() = %v, want nil for nonexistent", loaded)
	}
}

func TestDeleteSnapshot(t *testing.T) {
	setupSnapshotDir(t)

	snap := Snapshot{
		ItemID: "item1",
		Name:   "github1",
		Vars:   map[string]string{"KEY": "val"},
	}
	if err := SaveSnapshot(snap); err != nil {
		t.Fatalf("SaveSnapshot() error = %v", err)
	}

	if err := DeleteSnapshot("item1"); err != nil {
		t.Fatalf("DeleteSnapshot() error = %v", err)
	}

	loaded, err := LoadSnapshot("item1")
	if err != nil {
		t.Fatalf("LoadSnapshot() after delete error = %v", err)
	}
	if loaded != nil {
		t.Errorf("LoadSnapshot() after delete = %v, want nil", loaded)
	}
}

func TestDeleteSnapshot_NotExist(t *testing.T) {
	setupSnapshotDir(t)

	// Should not return error for nonexistent file
	if err := DeleteSnapshot("nonexistent"); err != nil {
		t.Errorf("DeleteSnapshot() error = %v, want nil", err)
	}
}

func TestListSnapshots(t *testing.T) {
	setupSnapshotDir(t)

	// Empty dir returns nil
	snaps, err := ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots() empty error = %v", err)
	}
	if snaps != nil {
		t.Errorf("ListSnapshots() empty = %v, want nil", snaps)
	}

	// Save two snapshots
	_ = SaveSnapshot(Snapshot{ItemID: "item1", Name: "github1", Vars: map[string]string{"A": "1"}})
	_ = SaveSnapshot(Snapshot{ItemID: "item2", Name: "github2", Vars: map[string]string{"B": "2"}})

	snaps, err = ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}
	if len(snaps) != 2 {
		t.Fatalf("ListSnapshots() got %d, want 2", len(snaps))
	}

	// Verify both are present (order not guaranteed)
	ids := map[string]bool{}
	for _, s := range snaps {
		ids[s.ItemID] = true
	}
	if !ids["item1"] || !ids["item2"] {
		t.Errorf("ListSnapshots() missing expected items, got IDs: %v", ids)
	}
}

func TestListSnapshots_IgnoresNonJSON(t *testing.T) {
	home := setupSnapshotDir(t)

	// Create snapshot dir with a non-json file
	dir := filepath.Join(home, snapshotDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not json"), 0o600)

	snaps, err := ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}
	if len(snaps) != 0 {
		t.Errorf("ListSnapshots() got %d, want 0 (should ignore non-json)", len(snaps))
	}
}

func TestSaveSnapshot_Overwrite(t *testing.T) {
	setupSnapshotDir(t)

	_ = SaveSnapshot(Snapshot{ItemID: "item1", Name: "github1", Vars: map[string]string{"TOKEN": "old"}})
	_ = SaveSnapshot(Snapshot{ItemID: "item1", Name: "github1", Vars: map[string]string{"TOKEN": "new"}})

	loaded, err := LoadSnapshot("item1")
	if err != nil {
		t.Fatalf("LoadSnapshot() error = %v", err)
	}
	if loaded.Vars["TOKEN"] != "new" {
		t.Errorf("Vars[TOKEN] = %q, want %q after overwrite", loaded.Vars["TOKEN"], "new")
	}
}

func TestDetectConflicts_NoConflict(t *testing.T) {
	setupSnapshotDir(t)

	_ = SaveSnapshot(Snapshot{ItemID: "item1", Name: "github1", Vars: map[string]string{"TOKEN": "aaa"}})

	vars := []EnvVar{{Key: "OTHER_KEY", Value: "bbb"}}
	conflicts, err := DetectConflicts(vars, "item2")
	if err != nil {
		t.Fatalf("DetectConflicts() error = %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("DetectConflicts() got %d conflicts, want 0", len(conflicts))
	}
}

func TestDetectConflicts_WithConflict(t *testing.T) {
	setupSnapshotDir(t)

	_ = SaveSnapshot(Snapshot{ItemID: "item1", Name: "github1", Vars: map[string]string{"TOKEN": "aaa"}})

	vars := []EnvVar{{Key: "TOKEN", Value: "bbb"}}
	conflicts, err := DetectConflicts(vars, "item2")
	if err != nil {
		t.Fatalf("DetectConflicts() error = %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("DetectConflicts() got %d conflicts, want 1", len(conflicts))
	}
	c := conflicts[0]
	if c.Key != "TOKEN" {
		t.Errorf("conflict Key = %q, want %q", c.Key, "TOKEN")
	}
	if c.ExistingName != "github1" {
		t.Errorf("conflict ExistingName = %q, want %q", c.ExistingName, "github1")
	}
	if c.ExistingVal != "aaa" {
		t.Errorf("conflict ExistingVal = %q, want %q", c.ExistingVal, "aaa")
	}
	if c.NewVal != "bbb" {
		t.Errorf("conflict NewVal = %q, want %q", c.NewVal, "bbb")
	}
}

func TestDetectConflicts_ExcludesSelf(t *testing.T) {
	setupSnapshotDir(t)

	_ = SaveSnapshot(Snapshot{ItemID: "item1", Name: "github1", Vars: map[string]string{"TOKEN": "aaa"}})

	// Same item ID should not conflict with itself (re-deploy scenario)
	vars := []EnvVar{{Key: "TOKEN", Value: "bbb"}}
	conflicts, err := DetectConflicts(vars, "item1")
	if err != nil {
		t.Fatalf("DetectConflicts() error = %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("DetectConflicts() got %d conflicts, want 0 (should exclude self)", len(conflicts))
	}
}

func TestDetectConflicts_MultipleVarsPartialConflict(t *testing.T) {
	setupSnapshotDir(t)

	_ = SaveSnapshot(Snapshot{ItemID: "item1", Name: "github1", Vars: map[string]string{"TOKEN": "aaa", "SECRET": "sss"}})

	vars := []EnvVar{
		{Key: "TOKEN", Value: "bbb"},
		{Key: "NEW_KEY", Value: "ccc"},
		{Key: "SECRET", Value: "ddd"},
	}
	conflicts, err := DetectConflicts(vars, "item2")
	if err != nil {
		t.Fatalf("DetectConflicts() error = %v", err)
	}
	if len(conflicts) != 2 {
		t.Fatalf("DetectConflicts() got %d conflicts, want 2", len(conflicts))
	}

	keys := map[string]bool{}
	for _, c := range conflicts {
		keys[c.Key] = true
	}
	if !keys["TOKEN"] || !keys["SECRET"] {
		t.Errorf("expected conflicts for TOKEN and SECRET, got: %v", keys)
	}
}

func TestDetectConflicts_NoSnapshots(t *testing.T) {
	setupSnapshotDir(t)

	vars := []EnvVar{{Key: "TOKEN", Value: "aaa"}}
	conflicts, err := DetectConflicts(vars, "item1")
	if err != nil {
		t.Fatalf("DetectConflicts() error = %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("DetectConflicts() got %d conflicts, want 0", len(conflicts))
	}
}

func TestFindRestorationValue_Found(t *testing.T) {
	setupSnapshotDir(t)

	_ = SaveSnapshot(Snapshot{ItemID: "item1", Name: "github1", Vars: map[string]string{"TOKEN": "aaa"}})
	_ = SaveSnapshot(Snapshot{ItemID: "item2", Name: "github2", Vars: map[string]string{"TOKEN": "bbb"}})

	// item2 is first in ordered list, so should be preferred
	val, name, found := FindRestorationValue("TOKEN", "item3", []string{"item2", "item1"})
	if !found {
		t.Fatal("FindRestorationValue() found = false, want true")
	}
	if val != "bbb" {
		t.Errorf("val = %q, want %q", val, "bbb")
	}
	if name != "github2" {
		t.Errorf("name = %q, want %q", name, "github2")
	}
}

func TestFindRestorationValue_ExcludesSelf(t *testing.T) {
	setupSnapshotDir(t)

	_ = SaveSnapshot(Snapshot{ItemID: "item1", Name: "github1", Vars: map[string]string{"TOKEN": "aaa"}})

	// Exclude item1, no other snapshot has TOKEN
	val, name, found := FindRestorationValue("TOKEN", "item1", []string{"item1"})
	if found {
		t.Errorf("FindRestorationValue() found = true (val=%q, name=%q), want false", val, name)
	}
}

func TestFindRestorationValue_FallbackOrder(t *testing.T) {
	setupSnapshotDir(t)

	_ = SaveSnapshot(Snapshot{ItemID: "item1", Name: "github1", Vars: map[string]string{"TOKEN": "aaa"}})
	_ = SaveSnapshot(Snapshot{ItemID: "item2", Name: "github2", Vars: map[string]string{"TOKEN": "bbb"}})

	// Exclude item2 (the most recent), should fall back to item1
	val, name, found := FindRestorationValue("TOKEN", "item2", []string{"item2", "item1"})
	if !found {
		t.Fatal("FindRestorationValue() found = false, want true")
	}
	if val != "aaa" {
		t.Errorf("val = %q, want %q (should fall back to item1)", val, "aaa")
	}
	if name != "github1" {
		t.Errorf("name = %q, want %q", name, "github1")
	}
}

func TestFindRestorationValue_KeyNotInAnySnapshot(t *testing.T) {
	setupSnapshotDir(t)

	_ = SaveSnapshot(Snapshot{ItemID: "item1", Name: "github1", Vars: map[string]string{"OTHER": "aaa"}})

	_, _, found := FindRestorationValue("TOKEN", "item2", []string{"item1"})
	if found {
		t.Error("FindRestorationValue() found = true, want false for missing key")
	}
}

func TestFindRestorationValue_NoSnapshots(t *testing.T) {
	setupSnapshotDir(t)

	_, _, found := FindRestorationValue("TOKEN", "item1", []string{"item2"})
	if found {
		t.Error("FindRestorationValue() found = true, want false with no snapshots")
	}
}
