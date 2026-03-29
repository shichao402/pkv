package env

import (
	"testing"
)

func TestParseEnvVars(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []EnvVar
		wantErr bool
	}{
		{
			name:    "empty string",
			content: "",
			want:    nil,
			wantErr: false,
		},
		{
			name:    "single KEY=VALUE",
			content: "FOO=bar",
			want:    []EnvVar{{Key: "FOO", Value: "bar"}},
			wantErr: false,
		},
		{
			name:    "multiple KEY=VALUE",
			content: "FOO=bar\nBAZ=qux",
			want:    []EnvVar{{Key: "FOO", Value: "bar"}, {Key: "BAZ", Value: "qux"}},
			wantErr: false,
		},
		{
			name:    "export KEY=VALUE format",
			content: "export FOO=bar",
			want:    []EnvVar{{Key: "FOO", Value: "bar"}},
			wantErr: false,
		},
		{
			name:    "double quoted value",
			content: `KEY="value"`,
			want:    []EnvVar{{Key: "KEY", Value: "value"}},
			wantErr: false,
		},
		{
			name:    "single quoted value",
			content: "KEY='value'",
			want:    []EnvVar{{Key: "KEY", Value: "value"}},
			wantErr: false,
		},
		{
			name:    "comment line",
			content: "# this is a comment",
			want:    nil,
			wantErr: false,
		},
		{
			name:    "empty lines skipped",
			content: "\n\n\n",
			want:    nil,
			wantErr: false,
		},
		{
			name:    "mixed comments and normal lines",
			content: "# comment\nFOO=bar\n# another comment\nBAZ=qux",
			want:    []EnvVar{{Key: "FOO", Value: "bar"}, {Key: "BAZ", Value: "qux"}},
			wantErr: false,
		},
		{
			name:    "error no equals sign",
			content: "INVALID_LINE",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "error equals at start",
			content: "=VALUE",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "value contains equals sign",
			content: "KEY=a=b",
			want:    []EnvVar{{Key: "KEY", Value: "a=b"}},
			wantErr: false,
		},
		{
			name:    "unmatched quotes preserved",
			content: `KEY="value`,
			want:    []EnvVar{{Key: "KEY", Value: `"value`}},
			wantErr: false,
		},
		{
			name:    "whitespace only value",
			content: "KEY=  ",
			want:    []EnvVar{{Key: "KEY", Value: ""}},
			wantErr: false,
		},
		{
			name:    "export with extra spaces",
			content: "export   FOO=bar",
			want:    []EnvVar{{Key: "FOO", Value: "bar"}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseEnvVars(tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEnvVars() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("ParseEnvVars() got %d vars, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i].Key != tt.want[i].Key || got[i].Value != tt.want[i].Value {
					t.Errorf("ParseEnvVars()[%d] = {%q, %q}, want {%q, %q}",
						i, got[i].Key, got[i].Value, tt.want[i].Key, tt.want[i].Value)
				}
			}
		})
	}
}

func TestStripQuotes(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		lineNum int
		want    string
	}{
		{
			name:    "double quotes",
			value:   `"value"`,
			lineNum: 1,
			want:    "value",
		},
		{
			name:    "single quotes",
			value:   "'value'",
			lineNum: 1,
			want:    "value",
		},
		{
			name:    "no quotes",
			value:   "value",
			lineNum: 1,
			want:    "value",
		},
		{
			name:    "empty string",
			value:   "",
			lineNum: 1,
			want:    "",
		},
		{
			name:    "single character",
			value:   "a",
			lineNum: 1,
			want:    "a",
		},
		{
			name:    "mismatched quotes",
			value:   `"value'`,
			lineNum: 1,
			want:    `"value'`,
		},
		{
			name:    "empty double quotes",
			value:   `""`,
			lineNum: 1,
			want:    "",
		},
		{
			name:    "empty single quotes",
			value:   "''",
			lineNum: 1,
			want:    "",
		},
		{
			name:    "only opening double quote",
			value:   `"value`,
			lineNum: 1,
			want:    `"value`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripQuotes(tt.value, tt.lineNum)
			if got != tt.want {
				t.Errorf("stripQuotes(%q, %d) = %q, want %q", tt.value, tt.lineNum, got, tt.want)
			}
		})
	}
}
