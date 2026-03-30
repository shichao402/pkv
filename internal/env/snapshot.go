package env

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const snapshotDir = ".pkv/env_snapshots"

// Snapshot records the full key-value set deployed by a single Bitwarden item.
type Snapshot struct {
	ItemID string            `json:"item_id"`
	Name   string            `json:"name"`
	Vars   map[string]string `json:"vars"`
}

func snapshotDirPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, snapshotDir), nil
}

func snapshotFilePath(itemID string) (string, error) {
	dir, err := snapshotDirPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, itemID+".json"), nil
}

// SaveSnapshot writes a snapshot file for the given item.
func SaveSnapshot(snap Snapshot) error {
	dir, err := snapshotDirPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path, err := snapshotFilePath(snap.ItemID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// LoadSnapshot reads a snapshot file for the given item ID.
// Returns nil if the file does not exist.
func LoadSnapshot(itemID string) (*Snapshot, error) {
	path, err := snapshotFilePath(itemID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

// DeleteSnapshot removes the snapshot file for the given item ID.
func DeleteSnapshot(itemID string) error {
	path, err := snapshotFilePath(itemID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ListSnapshots reads all snapshot files from the snapshot directory.
func ListSnapshots() ([]Snapshot, error) {
	dir, err := snapshotDirPath()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var snapshots []Snapshot
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var snap Snapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			continue
		}
		snapshots = append(snapshots, snap)
	}
	return snapshots, nil
}

// ConflictInfo describes a key conflict between a new deploy and an existing snapshot.
type ConflictInfo struct {
	Key          string
	ExistingName string // folder/item name that already owns this key
	ExistingVal  string
	NewVal       string
}

// DetectConflicts checks whether any of the given vars conflict with keys
// in existing snapshots belonging to other items (excluding excludeItemID).
func DetectConflicts(vars []EnvVar, excludeItemID string) ([]ConflictInfo, error) {
	snapshots, err := ListSnapshots()
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	var conflicts []ConflictInfo
	for _, v := range vars {
		for _, snap := range snapshots {
			if snap.ItemID == excludeItemID {
				continue
			}
			if existingVal, ok := snap.Vars[v.Key]; ok {
				conflicts = append(conflicts, ConflictInfo{
					Key:          v.Key,
					ExistingName: snap.Name,
					ExistingVal:  existingVal,
					NewVal:       v.Value,
				})
				break // only report first conflict per key
			}
		}
	}
	return conflicts, nil
}

// FindRestorationValue looks through other snapshots (excluding excludeItemID)
// for a value to restore for the given key.
// When multiple snapshots have the same key, the one with the most recent set_at
// (from state entries) should be preferred — the caller passes ordered item IDs.
// This function returns the first match from orderedItemIDs.
func FindRestorationValue(key string, excludeItemID string, orderedItemIDs []string) (string, string, bool) {
	for _, itemID := range orderedItemIDs {
		if itemID == excludeItemID {
			continue
		}
		snap, err := LoadSnapshot(itemID)
		if err != nil || snap == nil {
			continue
		}
		if val, ok := snap.Vars[key]; ok {
			return val, snap.Name, true
		}
	}
	return "", "", false
}
