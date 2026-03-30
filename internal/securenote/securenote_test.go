package securenote

import (
	"testing"

	"github.com/shichao402/pkv/internal/bw/types"
)

func TestAdd(t *testing.T) {
	tests := []struct {
		name    string
		isEnv   bool
		wantErr bool
	}{
		{"regular note", false, false},
		{"env note", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This would require mocking the bw.Client
			// Skipping for now as we need integration tests
			t.Skip("requires mocking bw.Client")
		})
	}
}

func TestResolveItem(t *testing.T) {
	items := []types.Item{
		{ID: "id-1", Name: "nginx.conf"},
		{ID: "id-2", Name: "app.env"},
		{ID: "id-3", Name: "config"},
	}

	tests := []struct {
		nameOrID    string
		wantID      string
		wantName    string
		wantErr     bool
		description string
	}{
		// Match by name
		{"nginx.conf", "id-1", "nginx.conf", false, "match by exact name"},
		{"app.env", "id-2", "app.env", false, "match by exact name 2"},

		// Match by ID
		{"id-1", "id-1", "nginx.conf", false, "match by id"},
		{"id-3", "id-3", "config", false, "match by id 3"},

		// No match
		{"nonexistent", "", "", true, "name not found"},
		{"id-999", "", "", true, "id not found"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			item, err := ResolveItem(items, tt.nameOrID)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if item.ID != tt.wantID {
				t.Errorf("ID: got %q, want %q", item.ID, tt.wantID)
			}
			if item.Name != tt.wantName {
				t.Errorf("Name: got %q, want %q", item.Name, tt.wantName)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes    int
		expected string
	}{
		{0, "0B"},
		{512, "512B"},
		{1023, "1023B"},
		{1024, "1.0KB"},
		{1536, "1.5KB"},
		{1048576, "1.0MB"},
		{5242880, "5.0MB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := FormatSize(tt.bytes)
			if got != tt.expected {
				t.Errorf("FormatSize(%d): got %q, want %q", tt.bytes, got, tt.expected)
			}
		})
	}
}

func TestCapitalizeFirst(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"a", "A"},
		{"abc", "Abc"},
		{"notes", "Notes"},
		{"Notes", "Notes"},
		{"123", "123"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := capitalizeFirst(tt.input)
			if got != tt.expected {
				t.Errorf("capitalizeFirst(%q): got %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
