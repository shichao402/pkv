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
