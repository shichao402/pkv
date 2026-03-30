package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const stateFileName = ".pkv/state.json"

type SSHKeyEntry struct {
	ItemID      string   `json:"item_id"`
	KeyName     string   `json:"key_name"`
	KeyFile     string   `json:"key_file"`
	PubFile     string   `json:"pub_file"`
	Hosts       []string `json:"hosts"`
	AddedAt     string   `json:"added_at"`
	Fingerprint string   `json:"fingerprint,omitempty"` // SHA256 fingerprint of stored SSH key
	StoredAt    string   `json:"stored_at,omitempty"`   // When key was stored in Bitwarden
}

type NoteEntry struct {
	ItemID   string `json:"item_id"`
	FileName string `json:"file_name"`
	FilePath string `json:"file_path"`
	SyncedAt string `json:"synced_at"`
}

type EnvEntry struct {
	ItemID string   `json:"item_id"`
	Name   string   `json:"name"`
	Keys   []string `json:"keys"`
	SetAt  string   `json:"set_at"`
}

type State struct {
	SSHKeys []SSHKeyEntry `json:"ssh_keys"`
	Notes   []NoteEntry   `json:"notes"`
	Envs    []EnvEntry    `json:"envs"`
	path    string
}

func statePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, stateFileName), nil
}

// Load reads the state file. Returns empty state if file doesn't exist.
func Load() (*State, error) {
	p, err := statePath()
	if err != nil {
		return nil, err
	}

	st := &State{path: p}

	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, st); err != nil {
		return nil, err
	}

	// Validate date fields are in RFC3339 format
	if err := validateDates(st); err != nil {
		return nil, err
	}

	return st, nil
}

// validateDates checks that all date fields are valid RFC3339 timestamps.
func validateDates(st *State) error {
	// Check SSH key dates
	for _, entry := range st.SSHKeys {
		if entry.AddedAt != "" {
			if _, err := time.Parse(time.RFC3339, entry.AddedAt); err != nil {
				return err
			}
		}
	}

	// Check note dates
	for _, entry := range st.Notes {
		if entry.SyncedAt != "" {
			if _, err := time.Parse(time.RFC3339, entry.SyncedAt); err != nil {
				return err
			}
		}
	}

	// Check env dates
	for _, entry := range st.Envs {
		if entry.SetAt != "" {
			if _, err := time.Parse(time.RFC3339, entry.SetAt); err != nil {
				return err
			}
		}
	}

	return nil
}

// Save writes the state to disk.
func (s *State) Save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

// AddSSHKey records a deployed SSH key.
func (s *State) AddSSHKey(entry SSHKeyEntry) {
	entry.AddedAt = time.Now().Format(time.RFC3339)
	// Replace existing entry for same item
	for i, e := range s.SSHKeys {
		if e.ItemID == entry.ItemID {
			s.SSHKeys[i] = entry
			return
		}
	}
	s.SSHKeys = append(s.SSHKeys, entry)
}

// AddEnv records deployed environment variables.
func (s *State) AddEnv(entry EnvEntry) {
	entry.SetAt = time.Now().Format(time.RFC3339)
	for i, e := range s.Envs {
		if e.ItemID == entry.ItemID {
			s.Envs[i] = entry
			return
		}
	}
	s.Envs = append(s.Envs, entry)
}

// FindEnvsByName returns all env entries matching the given folder/item name.
func (s *State) FindEnvsByName(name string) []EnvEntry {
	var matched []EnvEntry
	for _, e := range s.Envs {
		if e.Name == name {
			matched = append(matched, e)
		}
	}
	return matched
}

// RemoveEnvsByName removes all env entries matching the given folder/item name.
func (s *State) RemoveEnvsByName(name string) {
	var kept []EnvEntry
	for _, e := range s.Envs {
		if e.Name != name {
			kept = append(kept, e)
		}
	}
	s.Envs = kept
}

// EnvItemIDsByRecency returns item IDs from env entries sorted by SetAt descending (most recent first).
func (s *State) EnvItemIDsByRecency() []string {
	// Already stored in append order; sort by SetAt descending
	type ts struct {
		id    string
		setAt string
	}
	entries := make([]ts, 0, len(s.Envs))
	for _, e := range s.Envs {
		entries = append(entries, ts{id: e.ItemID, setAt: e.SetAt})
	}
	// Simple insertion sort (small list)
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].setAt > entries[j-1].setAt; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.id
	}
	return ids
}

// AddNote records a synced note.
func (s *State) AddNote(entry NoteEntry) {
	entry.SyncedAt = time.Now().Format(time.RFC3339)
	// Replace existing entry for same item
	for i, e := range s.Notes {
		if e.ItemID == entry.ItemID {
			s.Notes[i] = entry
			return
		}
	}
	s.Notes = append(s.Notes, entry)
}

// AddStoredSSHKey records a key stored in Bitwarden.
func (s *State) AddStoredSSHKey(itemID, keyName, fingerprint string) {
	entry := SSHKeyEntry{
		ItemID:      itemID,
		KeyName:     keyName,
		Fingerprint: fingerprint,
		StoredAt:    time.Now().Format(time.RFC3339),
	}
	// Replace existing entry for same item
	for i, e := range s.SSHKeys {
		if e.ItemID == itemID {
			s.SSHKeys[i] = entry
			return
		}
	}
	s.SSHKeys = append(s.SSHKeys, entry)
}

// FindStoredSSHKeyByFingerprint finds a stored SSH key by its fingerprint.
func (s *State) FindStoredSSHKeyByFingerprint(fingerprint string) *SSHKeyEntry {
	for i, e := range s.SSHKeys {
		if e.Fingerprint == fingerprint {
			return &s.SSHKeys[i]
		}
	}
	return nil
}

// RemoveStoredSSHKey removes a stored SSH key by itemID.
func (s *State) RemoveStoredSSHKey(itemID string) {
	var kept []SSHKeyEntry
	for _, e := range s.SSHKeys {
		if e.ItemID != itemID {
			kept = append(kept, e)
		}
	}
	s.SSHKeys = kept
}
