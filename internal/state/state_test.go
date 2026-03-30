package state

import (
	"testing"
	"time"
)

func TestValidateDates(t *testing.T) {
	validTime := time.Now().Format(time.RFC3339)

	tests := []struct {
		name    string
		state   *State
		wantErr bool
	}{
		{
			name: "all dates valid",
			state: &State{
				SSHKeys: []SSHKeyEntry{{ItemID: "1", AddedAt: validTime}},
				Notes:   []NoteEntry{{ItemID: "2", SyncedAt: validTime}},
				Envs:    []EnvEntry{{ItemID: "3", SetAt: validTime}},
			},
			wantErr: false,
		},
		{
			name: "all dates empty",
			state: &State{
				SSHKeys: []SSHKeyEntry{{ItemID: "1", AddedAt: ""}},
				Notes:   []NoteEntry{{ItemID: "2", SyncedAt: ""}},
				Envs:    []EnvEntry{{ItemID: "3", SetAt: ""}},
			},
			wantErr: false,
		},
		{
			name: "SSHKey invalid date",
			state: &State{
				SSHKeys: []SSHKeyEntry{{ItemID: "1", AddedAt: "not-a-date"}},
			},
			wantErr: true,
		},
		{
			name: "Note invalid date",
			state: &State{
				Notes: []NoteEntry{{ItemID: "2", SyncedAt: "bad-date"}},
			},
			wantErr: true,
		},
		{
			name: "Env invalid date",
			state: &State{
				Envs: []EnvEntry{{ItemID: "3", SetAt: "invalid"}},
			},
			wantErr: true,
		},
		{
			name:    "empty state",
			state:   &State{},
			wantErr: false,
		},
		{
			name: "mixed valid and empty dates",
			state: &State{
				SSHKeys: []SSHKeyEntry{
					{ItemID: "1", AddedAt: validTime},
					{ItemID: "2", AddedAt: ""},
				},
				Notes: []NoteEntry{{ItemID: "3", SyncedAt: ""}},
				Envs:  []EnvEntry{{ItemID: "4", SetAt: validTime}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDates(tt.state)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDates() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAddSSHKey(t *testing.T) {
	t.Run("add new entry", func(t *testing.T) {
		s := &State{}
		s.AddSSHKey(SSHKeyEntry{ItemID: "key1", KeyName: "mykey"})

		if len(s.SSHKeys) != 1 {
			t.Fatalf("expected 1 SSH key, got %d", len(s.SSHKeys))
		}
		if s.SSHKeys[0].ItemID != "key1" {
			t.Errorf("ItemID = %q, want %q", s.SSHKeys[0].ItemID, "key1")
		}
		if s.SSHKeys[0].KeyName != "mykey" {
			t.Errorf("KeyName = %q, want %q", s.SSHKeys[0].KeyName, "mykey")
		}
	})

	t.Run("update existing ItemID", func(t *testing.T) {
		s := &State{
			SSHKeys: []SSHKeyEntry{
				{ItemID: "key1", KeyName: "oldname"},
			},
		}
		s.AddSSHKey(SSHKeyEntry{ItemID: "key1", KeyName: "newname"})

		if len(s.SSHKeys) != 1 {
			t.Fatalf("expected 1 SSH key after update, got %d", len(s.SSHKeys))
		}
		if s.SSHKeys[0].KeyName != "newname" {
			t.Errorf("KeyName = %q, want %q after update", s.SSHKeys[0].KeyName, "newname")
		}
	})

	t.Run("AddedAt is auto set", func(t *testing.T) {
		s := &State{}
		before := time.Now().Add(-time.Second)
		s.AddSSHKey(SSHKeyEntry{ItemID: "key1"})
		after := time.Now().Add(time.Second)

		addedAt, err := time.Parse(time.RFC3339, s.SSHKeys[0].AddedAt)
		if err != nil {
			t.Fatalf("AddedAt is not valid RFC3339: %v", err)
		}
		if addedAt.Before(before) || addedAt.After(after) {
			t.Errorf("AddedAt = %v, expected between %v and %v", addedAt, before, after)
		}
	})
}

func TestAddEnv(t *testing.T) {
	t.Run("add new entry", func(t *testing.T) {
		s := &State{}
		s.AddEnv(EnvEntry{ItemID: "env1", Name: "myenv", Keys: []string{"KEY1"}})

		if len(s.Envs) != 1 {
			t.Fatalf("expected 1 env entry, got %d", len(s.Envs))
		}
		if s.Envs[0].ItemID != "env1" {
			t.Errorf("ItemID = %q, want %q", s.Envs[0].ItemID, "env1")
		}
	})

	t.Run("update existing ItemID", func(t *testing.T) {
		s := &State{
			Envs: []EnvEntry{
				{ItemID: "env1", Name: "old", Keys: []string{"OLD"}},
			},
		}
		s.AddEnv(EnvEntry{ItemID: "env1", Name: "new", Keys: []string{"NEW"}})

		if len(s.Envs) != 1 {
			t.Fatalf("expected 1 env entry after update, got %d", len(s.Envs))
		}
		if s.Envs[0].Name != "new" {
			t.Errorf("Name = %q, want %q after update", s.Envs[0].Name, "new")
		}
	})

	t.Run("SetAt is auto set", func(t *testing.T) {
		s := &State{}
		before := time.Now().Add(-time.Second)
		s.AddEnv(EnvEntry{ItemID: "env1"})
		after := time.Now().Add(time.Second)

		setAt, err := time.Parse(time.RFC3339, s.Envs[0].SetAt)
		if err != nil {
			t.Fatalf("SetAt is not valid RFC3339: %v", err)
		}
		if setAt.Before(before) || setAt.After(after) {
			t.Errorf("SetAt = %v, expected between %v and %v", setAt, before, after)
		}
	})
}

func TestAddNote(t *testing.T) {
	t.Run("add new entry", func(t *testing.T) {
		s := &State{}
		s.AddNote(NoteEntry{ItemID: "note1", FileName: "config.yml", FilePath: "/tmp/config.yml"})

		if len(s.Notes) != 1 {
			t.Fatalf("expected 1 note entry, got %d", len(s.Notes))
		}
		if s.Notes[0].ItemID != "note1" {
			t.Errorf("ItemID = %q, want %q", s.Notes[0].ItemID, "note1")
		}
	})

	t.Run("update existing ItemID", func(t *testing.T) {
		s := &State{
			Notes: []NoteEntry{
				{ItemID: "note1", FileName: "old.yml", FilePath: "/tmp/old.yml"},
			},
		}
		s.AddNote(NoteEntry{ItemID: "note1", FileName: "new.yml", FilePath: "/tmp/new.yml"})

		if len(s.Notes) != 1 {
			t.Fatalf("expected 1 note entry after update, got %d", len(s.Notes))
		}
		if s.Notes[0].FileName != "new.yml" {
			t.Errorf("FileName = %q, want %q after update", s.Notes[0].FileName, "new.yml")
		}
	})

	t.Run("SyncedAt is auto set", func(t *testing.T) {
		s := &State{}
		before := time.Now().Add(-time.Second)
		s.AddNote(NoteEntry{ItemID: "note1"})
		after := time.Now().Add(time.Second)

		syncedAt, err := time.Parse(time.RFC3339, s.Notes[0].SyncedAt)
		if err != nil {
			t.Fatalf("SyncedAt is not valid RFC3339: %v", err)
		}
		if syncedAt.Before(before) || syncedAt.After(after) {
			t.Errorf("SyncedAt = %v, expected between %v and %v", syncedAt, before, after)
		}
	})
}

func TestFindEnvsByName(t *testing.T) {
	t.Run("match single entry", func(t *testing.T) {
		s := &State{
			Envs: []EnvEntry{
				{ItemID: "1", Name: "github1", Keys: []string{"A"}},
				{ItemID: "2", Name: "github2", Keys: []string{"B"}},
			},
		}
		matched := s.FindEnvsByName("github1")
		if len(matched) != 1 {
			t.Fatalf("got %d, want 1", len(matched))
		}
		if matched[0].ItemID != "1" {
			t.Errorf("ItemID = %q, want %q", matched[0].ItemID, "1")
		}
	})

	t.Run("match multiple entries with same name", func(t *testing.T) {
		s := &State{
			Envs: []EnvEntry{
				{ItemID: "1", Name: "github1", Keys: []string{"A"}},
				{ItemID: "2", Name: "github1", Keys: []string{"B"}},
				{ItemID: "3", Name: "github2", Keys: []string{"C"}},
			},
		}
		matched := s.FindEnvsByName("github1")
		if len(matched) != 2 {
			t.Fatalf("got %d, want 2", len(matched))
		}
	})

	t.Run("no match", func(t *testing.T) {
		s := &State{
			Envs: []EnvEntry{
				{ItemID: "1", Name: "github1", Keys: []string{"A"}},
			},
		}
		matched := s.FindEnvsByName("nonexistent")
		if len(matched) != 0 {
			t.Errorf("got %d, want 0", len(matched))
		}
	})

	t.Run("empty state", func(t *testing.T) {
		s := &State{}
		matched := s.FindEnvsByName("anything")
		if len(matched) != 0 {
			t.Errorf("got %d, want 0", len(matched))
		}
	})
}

func TestRemoveEnvsByName(t *testing.T) {
	t.Run("remove matching entries", func(t *testing.T) {
		s := &State{
			Envs: []EnvEntry{
				{ItemID: "1", Name: "github1", Keys: []string{"A"}},
				{ItemID: "2", Name: "github2", Keys: []string{"B"}},
				{ItemID: "3", Name: "github1", Keys: []string{"C"}},
			},
		}
		s.RemoveEnvsByName("github1")

		if len(s.Envs) != 1 {
			t.Fatalf("got %d entries, want 1", len(s.Envs))
		}
		if s.Envs[0].ItemID != "2" {
			t.Errorf("remaining ItemID = %q, want %q", s.Envs[0].ItemID, "2")
		}
	})

	t.Run("remove nonexistent name is no-op", func(t *testing.T) {
		s := &State{
			Envs: []EnvEntry{
				{ItemID: "1", Name: "github1", Keys: []string{"A"}},
			},
		}
		s.RemoveEnvsByName("nonexistent")

		if len(s.Envs) != 1 {
			t.Fatalf("got %d entries, want 1", len(s.Envs))
		}
	})

	t.Run("remove all entries", func(t *testing.T) {
		s := &State{
			Envs: []EnvEntry{
				{ItemID: "1", Name: "github1", Keys: []string{"A"}},
				{ItemID: "2", Name: "github1", Keys: []string{"B"}},
			},
		}
		s.RemoveEnvsByName("github1")

		if len(s.Envs) != 0 {
			t.Errorf("got %d entries, want 0", len(s.Envs))
		}
	})

	t.Run("empty state", func(t *testing.T) {
		s := &State{}
		s.RemoveEnvsByName("anything")
		if s.Envs != nil {
			t.Errorf("Envs = %v, want nil", s.Envs)
		}
	})
}

func TestEnvItemIDsByRecency(t *testing.T) {
	t.Run("sorted by SetAt descending", func(t *testing.T) {
		s := &State{
			Envs: []EnvEntry{
				{ItemID: "oldest", SetAt: "2025-01-01T00:00:00Z"},
				{ItemID: "newest", SetAt: "2025-03-01T00:00:00Z"},
				{ItemID: "middle", SetAt: "2025-02-01T00:00:00Z"},
			},
		}
		ids := s.EnvItemIDsByRecency()

		if len(ids) != 3 {
			t.Fatalf("got %d IDs, want 3", len(ids))
		}
		if ids[0] != "newest" {
			t.Errorf("ids[0] = %q, want %q", ids[0], "newest")
		}
		if ids[1] != "middle" {
			t.Errorf("ids[1] = %q, want %q", ids[1], "middle")
		}
		if ids[2] != "oldest" {
			t.Errorf("ids[2] = %q, want %q", ids[2], "oldest")
		}
	})

	t.Run("single entry", func(t *testing.T) {
		s := &State{
			Envs: []EnvEntry{
				{ItemID: "only", SetAt: "2025-01-01T00:00:00Z"},
			},
		}
		ids := s.EnvItemIDsByRecency()

		if len(ids) != 1 || ids[0] != "only" {
			t.Errorf("got %v, want [only]", ids)
		}
	})

	t.Run("empty state", func(t *testing.T) {
		s := &State{}
		ids := s.EnvItemIDsByRecency()

		if len(ids) != 0 {
			t.Errorf("got %v, want empty", ids)
		}
	})

	t.Run("same timestamp preserves order", func(t *testing.T) {
		s := &State{
			Envs: []EnvEntry{
				{ItemID: "a", SetAt: "2025-01-01T00:00:00Z"},
				{ItemID: "b", SetAt: "2025-01-01T00:00:00Z"},
			},
		}
		ids := s.EnvItemIDsByRecency()

		if len(ids) != 2 {
			t.Fatalf("got %d IDs, want 2", len(ids))
		}
		// Insertion sort is stable, so original order preserved for equal keys
		if ids[0] != "a" || ids[1] != "b" {
			t.Errorf("got %v, want [a b] (stable order)", ids)
		}
	})
}

func TestRemoveNote(t *testing.T) {
	tests := []struct {
		name      string
		initialID string
		removeID  string
		wantCount int
	}{
		{
			name:      "remove existing note",
			initialID: "note-1",
			removeID:  "note-1",
			wantCount: 0,
		},
		{
			name:      "remove non-existent note leaves state unchanged",
			initialID: "note-1",
			removeID:  "note-2",
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &State{
				Notes: []NoteEntry{{ItemID: tt.initialID, FileName: "test.txt"}},
			}

			s.RemoveNote(tt.removeID)

			if len(s.Notes) != tt.wantCount {
				t.Errorf("RemoveNote() left %d notes, want %d", len(s.Notes), tt.wantCount)
			}
		})
	}
}

func TestRemoveEnvByItemID(t *testing.T) {
	tests := []struct {
		name       string
		initialID  string
		removeID   string
		wantCount  int
		description string
	}{
		{
			name:        "remove existing env",
			initialID:   "env-1",
			removeID:    "env-1",
			wantCount:   0,
			description: "removes the matching env entry",
		},
		{
			name:        "remove non-existent env leaves state unchanged",
			initialID:   "env-1",
			removeID:    "env-2",
			wantCount:   1,
			description: "non-existent ID leaves state intact",
		},
		{
			name:        "remove from multiple envs",
			initialID:   "env-1",
			removeID:    "env-1",
			wantCount:   0,
			description: "removes correct entry from list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			s := &State{
				Envs: []EnvEntry{
					{ItemID: tt.initialID, Name: "github", Keys: []string{"TOKEN"}},
					{ItemID: "env-2", Name: "app", Keys: []string{"SECRET"}},
				},
			}

			initialCount := len(s.Envs)
			s.RemoveEnvByItemID(tt.removeID)

			if len(s.Envs) > initialCount {
				t.Errorf("RemoveEnvByItemID() increased env count")
			}

			for _, env := range s.Envs {
				if env.ItemID == tt.removeID {
					t.Errorf("RemoveEnvByItemID() did not remove entry with ID %s", tt.removeID)
				}
			}
		})
	}
}

func TestRemoveNoteMultiple(t *testing.T) {
	t.Run("remove from multiple notes", func(t *testing.T) {
		s := &State{
			Notes: []NoteEntry{
				{ItemID: "note-1", FileName: "file1.txt"},
				{ItemID: "note-2", FileName: "file2.txt"},
				{ItemID: "note-3", FileName: "file3.txt"},
			},
		}

		s.RemoveNote("note-2")

		if len(s.Notes) != 2 {
			t.Errorf("RemoveNote() left %d notes, want 2", len(s.Notes))
		}

		for _, note := range s.Notes {
			if note.ItemID == "note-2" {
				t.Error("RemoveNote() did not remove the target entry")
			}
		}

		// Verify order is preserved
		if s.Notes[0].ItemID != "note-1" || s.Notes[1].ItemID != "note-3" {
			t.Errorf("RemoveNote() corrupted note order")
		}
	})
}
