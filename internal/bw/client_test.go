package bw

import (
	"testing"

	"github.com/shichao402/pkv/internal/bw/types"
)

func TestFilterSSHKeys(t *testing.T) {
	tests := []struct {
		name   string
		items  []types.Item
		expect int
	}{
		{
			name: "mixed types returns only SSH keys",
			items: []types.Item{
				{ID: "1", Type: types.ItemTypeSSHKey, Name: "key1"},
				{ID: "2", Type: types.ItemTypeSecureNote, Name: "note1"},
				{ID: "3", Type: types.ItemTypeLogin, Name: "login1"},
				{ID: "4", Type: types.ItemTypeSSHKey, Name: "key2"},
			},
			expect: 2,
		},
		{
			name: "no SSH keys",
			items: []types.Item{
				{ID: "1", Type: types.ItemTypeSecureNote},
				{ID: "2", Type: types.ItemTypeLogin},
			},
			expect: 0,
		},
		{
			name:   "empty list",
			items:  []types.Item{},
			expect: 0,
		},
		{
			name: "all SSH keys",
			items: []types.Item{
				{ID: "1", Type: types.ItemTypeSSHKey, Name: "key1"},
				{ID: "2", Type: types.ItemTypeSSHKey, Name: "key2"},
				{ID: "3", Type: types.ItemTypeSSHKey, Name: "key3"},
			},
			expect: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterSSHKeys(tt.items)
			if len(got) != tt.expect {
				t.Errorf("FilterSSHKeys() returned %d items, want %d", len(got), tt.expect)
			}
			for _, item := range got {
				if item.Type != types.ItemTypeSSHKey {
					t.Errorf("FilterSSHKeys() returned item with type %d, want %d", item.Type, types.ItemTypeSSHKey)
				}
			}
		})
	}
}

func TestFilterSecureNotes(t *testing.T) {
	tests := []struct {
		name   string
		items  []types.Item
		expect int
	}{
		{
			name: "mixed types returns only secure notes",
			items: []types.Item{
				{ID: "1", Type: types.ItemTypeSecureNote, Name: "note1"},
				{ID: "2", Type: types.ItemTypeLogin, Name: "login1"},
				{ID: "3", Type: types.ItemTypeSSHKey, Name: "key1"},
				{ID: "4", Type: types.ItemTypeSecureNote, Name: "note2"},
			},
			expect: 2,
		},
		{
			name: "no secure notes",
			items: []types.Item{
				{ID: "1", Type: types.ItemTypeSSHKey},
				{ID: "2", Type: types.ItemTypeLogin},
			},
			expect: 0,
		},
		{
			name:   "empty list",
			items:  []types.Item{},
			expect: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterSecureNotes(tt.items)
			if len(got) != tt.expect {
				t.Errorf("FilterSecureNotes() returned %d items, want %d", len(got), tt.expect)
			}
			for _, item := range got {
				if item.Type != types.ItemTypeSecureNote {
					t.Errorf("FilterSecureNotes() returned item with type %d, want %d", item.Type, types.ItemTypeSecureNote)
				}
			}
		})
	}
}

func TestFilterEnvNotes(t *testing.T) {
	envField := types.CustomField{Name: types.PKVFieldName, Value: types.PKVTypeEnv}
	otherField := types.CustomField{Name: types.PKVFieldName, Value: "other"}

	tests := []struct {
		name          string
		items         []types.Item
		expectMatched int
		expectSkipped int
	}{
		{
			name: "env notes and non-env notes separated",
			items: []types.Item{
				{ID: "1", Type: types.ItemTypeSecureNote, Fields: []types.CustomField{envField}},
				{ID: "2", Type: types.ItemTypeSecureNote, Fields: []types.CustomField{otherField}},
				{ID: "3", Type: types.ItemTypeSecureNote}, // no pkv_type field
			},
			expectMatched: 1,
			expectSkipped: 2,
		},
		{
			name: "all env notes",
			items: []types.Item{
				{ID: "1", Type: types.ItemTypeSecureNote, Fields: []types.CustomField{envField}},
				{ID: "2", Type: types.ItemTypeSecureNote, Fields: []types.CustomField{envField}},
			},
			expectMatched: 2,
			expectSkipped: 0,
		},
		{
			name: "no env notes",
			items: []types.Item{
				{ID: "1", Type: types.ItemTypeSecureNote},
				{ID: "2", Type: types.ItemTypeSecureNote, Fields: []types.CustomField{otherField}},
			},
			expectMatched: 0,
			expectSkipped: 2,
		},
		{
			name: "non-SecureNote types are completely skipped",
			items: []types.Item{
				{ID: "1", Type: types.ItemTypeLogin},
				{ID: "2", Type: types.ItemTypeSSHKey},
				{ID: "3", Type: types.ItemTypeCard},
			},
			expectMatched: 0,
			expectSkipped: 0,
		},
		{
			name:          "empty list",
			items:         []types.Item{},
			expectMatched: 0,
			expectSkipped: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, skipped := FilterEnvNotes(tt.items)
			if len(matched) != tt.expectMatched {
				t.Errorf("FilterEnvNotes() matched = %d, want %d", len(matched), tt.expectMatched)
			}
			if len(skipped) != tt.expectSkipped {
				t.Errorf("FilterEnvNotes() skipped = %d, want %d", len(skipped), tt.expectSkipped)
			}
		})
	}
}

func TestFilterNonEnvNotes(t *testing.T) {
	envField := types.CustomField{Name: types.PKVFieldName, Value: types.PKVTypeEnv}

	tests := []struct {
		name   string
		items  []types.Item
		expect int
	}{
		{
			name: "mixed types returns SecureNote non-env only",
			items: []types.Item{
				{ID: "1", Type: types.ItemTypeSecureNote, Fields: []types.CustomField{envField}},
				{ID: "2", Type: types.ItemTypeSecureNote, Name: "plain note"},
				{ID: "3", Type: types.ItemTypeLogin},
				{ID: "4", Type: types.ItemTypeSecureNote, Name: "another note"},
			},
			expect: 2,
		},
		{
			name: "all env returns nil",
			items: []types.Item{
				{ID: "1", Type: types.ItemTypeSecureNote, Fields: []types.CustomField{envField}},
				{ID: "2", Type: types.ItemTypeSecureNote, Fields: []types.CustomField{envField}},
			},
			expect: 0,
		},
		{
			name:   "empty list",
			items:  []types.Item{},
			expect: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterNonEnvNotes(tt.items)
			if len(got) != tt.expect {
				t.Errorf("FilterNonEnvNotes() returned %d items, want %d", len(got), tt.expect)
			}
			for _, item := range got {
				if item.Type != types.ItemTypeSecureNote {
					t.Errorf("FilterNonEnvNotes() returned non-SecureNote type %d", item.Type)
				}
				if item.IsEnv() {
					t.Error("FilterNonEnvNotes() returned an env item")
				}
			}
		})
	}
}
