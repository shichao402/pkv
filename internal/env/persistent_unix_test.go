//go:build !windows

package env

import (
	"testing"
)

func TestEscapeShellQuote(t *testing.T) {
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
			input: "don't",
			want:  "don'\\''t",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "only single quote",
			input: "'",
			want:  "'\\''",
		},
		{
			name:  "multiple single quotes",
			input: "it's a test's",
			want:  "it'\\''s a test'\\''s",
		},
		{
			name:  "double quotes unchanged",
			input: `"hello"`,
			want:  `"hello"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeShellQuote(tt.input)
			if got != tt.want {
				t.Errorf("escapeShellQuote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidEnvVarName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "MY_VAR",
			input: "MY_VAR",
			want:  true,
		},
		{
			name:  "_VAR",
			input: "_VAR",
			want:  true,
		},
		{
			name:  "VAR123",
			input: "VAR123",
			want:  true,
		},
		{
			name:  "single letter",
			input: "a",
			want:  true,
		},
		{
			name:  "underscore with digits",
			input: "_123",
			want:  true,
		},
		{
			name:  "starts with digit",
			input: "123VAR",
			want:  false,
		},
		{
			name:  "starts with hyphen",
			input: "-VAR",
			want:  false,
		},
		{
			name:  "contains hyphen",
			input: "MY-VAR",
			want:  false,
		},
		{
			name:  "contains space",
			input: "MY VAR",
			want:  false,
		},
		{
			name:  "empty string",
			input: "",
			want:  false,
		},
		{
			name:  "starts with dollar sign",
			input: "$VAR",
			want:  false,
		},
		{
			name:  "contains dot",
			input: "A.B",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidEnvVarName(tt.input)
			if got != tt.want {
				t.Errorf("isValidEnvVarName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
