package bw

import (
	"io"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseBWVersion(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    string
		wantErr string
	}{
		{
			name:   "plain version",
			output: "2026.2.0\n",
			want:   "2026.2.0",
		},
		{
			name:   "prefixed version",
			output: "Bitwarden CLI v2026.2.0",
			want:   "2026.2.0",
		},
		{
			name:    "empty output",
			output:  "   ",
			wantErr: "empty output",
		},
		{
			name:    "unexpected output",
			output:  "Bitwarden CLI",
			wantErr: "unexpected output",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBWVersion(tt.output)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("parseBWVersion() expected error")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseBWVersion() error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("parseBWVersion() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseBWVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckBWInstalledReturnsMissingBinaryError(t *testing.T) {
	err := checkBWInstalled(func(string) (string, error) {
		return "", exec.ErrNotFound
	}, nil, io.Discard)
	if err == nil {
		t.Fatal("checkBWInstalled() expected error")
	}
	if !strings.Contains(err.Error(), "not found in PATH") {
		t.Fatalf("checkBWInstalled() error = %v, want missing binary context", err)
	}
}

func TestCheckBWInstalledReturnsVersionProbeFailure(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "bw.log")
	err := checkBWInstalled(
		func(string) (string, error) { return "/usr/local/bin/bw", nil },
		newTestBWExecCommand(t, "version_command_fails", logPath),
		io.Discard,
	)
	if err == nil {
		t.Fatal("checkBWInstalled() expected error")
	}
	if !strings.Contains(err.Error(), "failed version check") || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("checkBWInstalled() error = %v, want version probe failure context", err)
	}

	if got := readTestBWCalls(t, logPath); !reflect.DeepEqual(got, []string{
		"bw --version|env=",
	}) {
		t.Fatalf("bw calls = %#v", got)
	}
}

func TestCheckBWInstalledReturnsMalformedVersionOutputError(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "bw.log")
	err := checkBWInstalled(
		func(string) (string, error) { return "/usr/local/bin/bw", nil },
		newTestBWExecCommand(t, "version_malformed_output", logPath),
		io.Discard,
	)
	if err == nil {
		t.Fatal("checkBWInstalled() expected error")
	}
	if !strings.Contains(err.Error(), "unexpected output") {
		t.Fatalf("checkBWInstalled() error = %v, want malformed output context", err)
	}

	if got := readTestBWCalls(t, logPath); !reflect.DeepEqual(got, []string{
		"bw --version|env=",
	}) {
		t.Fatalf("bw calls = %#v", got)
	}
}
