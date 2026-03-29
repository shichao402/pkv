//go:build windows

package env

import (
	"testing"
)

func TestEscapePS(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no special characters",
			input: "hello",
			want:  "hello",
		},
		{
			name:  "contains single quote",
			input: "it's",
			want:  "it''s",
		},
		{
			name:  "contains dollar sign",
			input: "$HOME",
			want:  "`$HOME",
		},
		{
			name:  "contains backtick",
			input: "test`cmd",
			want:  "test``cmd",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "mixed special characters",
			input: "$var's`test",
			want:  "`$var''s``test",
		},
		{
			name:  "multiple single quotes",
			input: "a''b",
			want:  "a''''b",
		},
		{
			name:  "only special characters",
			input: "$`'",
			want:  "`$``''",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapePS(tt.input)
			if got != tt.want {
				t.Errorf("escapePS(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
