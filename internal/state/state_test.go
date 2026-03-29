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
