package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const stateFileName = ".pkv/state.json"

type SSHKeyEntry struct {
	ItemID  string   `json:"item_id"`
	KeyName string   `json:"key_name"`
	KeyFile string   `json:"key_file"`
	PubFile string   `json:"pub_file"`
	Hosts   []string `json:"hosts"`
	AddedAt string   `json:"added_at"`
}

type NoteEntry struct {
	ItemID   string `json:"item_id"`
	FileName string `json:"file_name"`
	FilePath string `json:"file_path"`
	SyncedAt string `json:"synced_at"`
}

type State struct {
	SSHKeys []SSHKeyEntry `json:"ssh_keys"`
	Notes   []NoteEntry   `json:"notes"`
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
	return st, nil
}

// Save writes the state to disk.
func (s *State) Save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
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
