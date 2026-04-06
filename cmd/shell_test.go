package cmd

import (
	"reflect"
	"testing"
)

func TestParseShellArgs(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    []string
		wantErr bool
	}{
		{
			name: "simple command",
			line: "prod ssh",
			want: []string{"prod", "ssh"},
		},
		{
			name: "quoted args",
			line: `prod note add --name "app secrets.json" --file './secrets file.json'`,
			want: []string{"prod", "note", "add", "--name", "app secrets.json", "--file", "./secrets file.json"},
		},
		{
			name: "escaped spaces",
			line: `prod note add --file ./secrets\ file.json`,
			want: []string{"prod", "note", "add", "--file", "./secrets file.json"},
		},
		{
			name: "pkv prefix",
			line: "pkv get prod ssh",
			want: []string{"pkv", "get", "prod", "ssh"},
		},
		{
			name:    "unterminated quote",
			line:    `prod note add --name "oops`,
			wantErr: true,
		},
		{
			name:    "unterminated escape",
			line:    `prod note add --file ./secrets\`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseShellArgs(tt.line)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseShellArgs() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseShellArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestTranslateShellArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    []string
		wantErr bool
	}{
		{
			name: "folder list",
			args: []string{"prod", "list"},
			want: []string{"list", "prod"},
		},
		{
			name: "implicit get ssh",
			args: []string{"prod", "ssh"},
			want: []string{"get", "prod", "ssh"},
		},
		{
			name: "explicit clean env",
			args: []string{"prod", "env", "clean"},
			want: []string{"clean", "prod", "env"},
		},
		{
			name: "note add passthrough",
			args: []string{"prod", "note", "add", "--name", "app.secrets.json"},
			want: []string{"add", "prod", "note", "--name", "app.secrets.json"},
		},
		{
			name:    "too short",
			args:    []string{"prod"},
			wantErr: true,
		},
		{
			name:    "unknown action",
			args:    []string{"prod", "note", "sync"},
			wantErr: true,
		},
		{
			name:    "unknown command",
			args:    []string{"prod", "sync"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := translateShellArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("translateShellArgs() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("translateShellArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestResetShellCommandState(t *testing.T) {
	addSSHPrivFlag = "priv"
	addSSHPubFlag = "pub"
	addNameFlag = "name"
	addNoteFileFlag = "file"

	resetShellCommandState()

	if addSSHPrivFlag != "" || addSSHPubFlag != "" || addNameFlag != "" || addNoteFileFlag != "" {
		t.Fatal("expected shell command flags to be reset")
	}
}
