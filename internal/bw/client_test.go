package bw

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
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

func TestBaseEncode(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantLen int
	}{
		{
			name:    "empty bytes",
			input:   []byte{},
			wantLen: 0,
		},
		{
			name:    "simple string",
			input:   []byte("hello"),
			wantLen: 8, // base64 of "hello" is 8 chars
		},
		{
			name:    "json-like string",
			input:   []byte(`{"type":2,"name":"test"}`),
			wantLen: 32, // base64 encoded length
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := base64Encode(tt.input)
			if len(encoded) != tt.wantLen {
				t.Errorf("base64Encode() length = %d, want %d", len(encoded), tt.wantLen)
			}
		})
	}
}

func TestEnsureUnlockedReusesExportedSession(t *testing.T) {
	t.Setenv("BW_SESSION", "valid-session")
	logPath := filepath.Join(t.TempDir(), "bw.log")

	client := NewClient()
	client.execCommand = newTestBWExecCommand(t, "reuse_exported_session", logPath)
	client.lookPath = func(string) (string, error) { return "/usr/local/bin/bw", nil }

	session, err := client.EnsureUnlocked()
	if err != nil {
		t.Fatalf("EnsureUnlocked() error = %v", err)
	}
	if session != "valid-session" {
		t.Fatalf("EnsureUnlocked() session = %q, want %q", session, "valid-session")
	}

	if got := readTestBWCalls(t, logPath); !reflect.DeepEqual(got, []string{
		"bw --version|env=valid-session",
		"bw --nointeraction --session valid-session list folders|env=valid-session",
	}) {
		t.Fatalf("bw calls = %#v", got)
	}
}

func TestEnsureUnlockedRefreshesExpiredExportedSession(t *testing.T) {
	t.Setenv("BW_SESSION", "expired-session")
	logPath := filepath.Join(t.TempDir(), "bw.log")

	client := NewClient()
	client.execCommand = newTestBWExecCommand(t, "refresh_expired_session", logPath)
	client.lookPath = func(string) (string, error) { return "/usr/local/bin/bw", nil }

	session, err := client.EnsureUnlocked()
	if err != nil {
		t.Fatalf("EnsureUnlocked() error = %v", err)
	}
	if session != "fresh-session" {
		t.Fatalf("EnsureUnlocked() session = %q, want %q", session, "fresh-session")
	}
	if got := os.Getenv("BW_SESSION"); got != "fresh-session" {
		t.Fatalf("BW_SESSION = %q, want %q", got, "fresh-session")
	}

	if got := readTestBWCalls(t, logPath); !reflect.DeepEqual(got, []string{
		"bw --version|env=expired-session",
		"bw --nointeraction --session expired-session list folders|env=expired-session",
		"bw --nointeraction status|env=",
		"bw unlock --raw|env=",
	}) {
		t.Fatalf("bw calls = %#v", got)
	}
}

func TestEnsureUnlockedReturnsExportedSessionValidationError(t *testing.T) {
	t.Setenv("BW_SESSION", "flaky-session")
	logPath := filepath.Join(t.TempDir(), "bw.log")

	client := NewClient()
	client.execCommand = newTestBWExecCommand(t, "exported_session_network_error", logPath)
	client.lookPath = func(string) (string, error) { return "/usr/local/bin/bw", nil }

	_, err := client.EnsureUnlocked()
	if err == nil {
		t.Fatal("EnsureUnlocked() expected error")
	}
	if !strings.Contains(err.Error(), "validate exported BW_SESSION") {
		t.Fatalf("EnsureUnlocked() error = %v, want exported session validation context", err)
	}

	if got := readTestBWCalls(t, logPath); !reflect.DeepEqual(got, []string{
		"bw --version|env=flaky-session",
		"bw --nointeraction --session flaky-session list folders|env=flaky-session",
	}) {
		t.Fatalf("bw calls = %#v", got)
	}
}

func newTestBWExecCommand(t *testing.T, scenario, logPath string) execCommandFunc {
	t.Helper()
	return func(name string, args ...string) *exec.Cmd {
		cmdArgs := append([]string{"-test.run=TestClientHelperProcess", "--", name}, args...)
		cmd := exec.Command(os.Args[0], cmdArgs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"PKV_TEST_BW_SCENARIO="+scenario,
			"PKV_TEST_BW_LOG="+logPath,
		)
		return cmd
	}
}

func readTestBWCalls(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read bw log: %v", err)
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func TestClientHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	args := os.Args
	sep := -1
	for i, arg := range args {
		if arg == "--" {
			sep = i
			break
		}
	}
	if sep == -1 || sep+1 >= len(args) {
		fmt.Fprintln(os.Stderr, "missing helper args")
		os.Exit(2)
	}

	bwArgs := args[sep+1:]
	if bwArgs[0] != "bw" {
		fmt.Fprintf(os.Stderr, "unexpected command: %q\n", bwArgs[0])
		os.Exit(2)
	}

	logPath := os.Getenv("PKV_TEST_BW_LOG")
	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open log: %v\n", err)
			os.Exit(2)
		}
		_, _ = fmt.Fprintf(f, "bw %s|env=%s\n", strings.Join(bwArgs[1:], " "), os.Getenv("BW_SESSION"))
		_ = f.Close()
	}

	joined := strings.Join(bwArgs[1:], " ")
	if joined == "--version" {
		switch os.Getenv("PKV_TEST_BW_SCENARIO") {
		case "version_command_fails":
			fmt.Fprint(os.Stderr, "permission denied\n")
			os.Exit(1)
		case "version_malformed_output":
			fmt.Fprint(os.Stdout, "Bitwarden CLI\n")
			os.Exit(0)
		default:
			fmt.Fprint(os.Stdout, "2026.2.0\n")
			os.Exit(0)
		}
	}

	switch os.Getenv("PKV_TEST_BW_SCENARIO") {
	case "reuse_exported_session":
		if joined == "--nointeraction --session valid-session list folders" {
			fmt.Fprint(os.Stdout, `[{"id":"folder-1","name":"dev"}]`)
			os.Exit(0)
		}
	case "refresh_expired_session":
		switch joined {
		case "--nointeraction --session expired-session list folders":
			fmt.Fprint(os.Stderr, "Vault is locked.\n")
			os.Exit(1)
		case "--nointeraction status":
			fmt.Fprint(os.Stdout, `{"status":"locked","userEmail":"dev@example.com"}`)
			os.Exit(0)
		case "unlock --raw":
			fmt.Fprint(os.Stdout, "fresh-session\n")
			os.Exit(0)
		}
	case "exported_session_network_error":
		if joined == "--nointeraction --session flaky-session list folders" {
			fmt.Fprint(os.Stderr, "network unreachable\n")
			os.Exit(1)
		}
	}

	fmt.Fprintf(os.Stderr, "unexpected bw args for %s: %q\n", os.Getenv("PKV_TEST_BW_SCENARIO"), joined)
	os.Exit(2)
}
