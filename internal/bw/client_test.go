package bw

import (
	"encoding/base64"
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

func TestFilterConfigNotes_ExcludesReservedMetadata(t *testing.T) {
	envField := types.CustomField{Name: types.PKVFieldName, Value: types.PKVTypeEnv}
	items := []types.Item{
		{ID: "1", Type: types.ItemTypeSecureNote, Name: types.ReservedEnvNoteName, Fields: []types.CustomField{envField}},
		{ID: "2", Type: types.ItemTypeSecureNote, Name: types.ReservedIncludeNoteName},
		{ID: "3", Type: types.ItemTypeSecureNote, Name: "app.env.json"},
		{ID: "4", Type: types.ItemTypeSecureNote, Name: "notes/readme.md"},
		{ID: "5", Type: types.ItemTypeLogin, Name: "login"},
	}

	got := FilterConfigNotes(items)
	if len(got) != 2 {
		t.Fatalf("FilterConfigNotes() returned %d items, want 2", len(got))
	}
	names := map[string]bool{}
	for _, item := range got {
		if item.Name == types.ReservedEnvNoteName || item.Name == types.ReservedIncludeNoteName {
			t.Errorf("FilterConfigNotes() returned reserved metadata note %q", item.Name)
		}
		names[item.Name] = true
	}
	if !names["app.env.json"] || !names["notes/readme.md"] {
		t.Errorf("FilterConfigNotes() missing config notes; got %v", names)
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
	resetBWInstalledCacheForTest()
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
	resetBWInstalledCacheForTest()
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
		"bw --raw unlock|env=",
		"bw --nointeraction --session fresh-session list folders|env=fresh-session",
	}) {
		t.Fatalf("bw calls = %#v", got)
	}
}

func TestParseUnlockSessionUsesLastField(t *testing.T) {
	got, err := parseUnlockSession([]byte("? Master password: fresh-session\n"))
	if err != nil {
		t.Fatalf("parseUnlockSession() error = %v, want nil", err)
	}
	if got != "fresh-session" {
		t.Fatalf("parseUnlockSession() = %q, want fresh-session", got)
	}
}

func TestParseUnlockSessionStripsANSISequences(t *testing.T) {
	got, err := parseUnlockSession([]byte("\x1b[?25h" + "fresh-session" + "\x1b[?25l\n"))
	if err != nil {
		t.Fatalf("parseUnlockSession() error = %v, want nil", err)
	}
	if got != "fresh-session" {
		t.Fatalf("parseUnlockSession() = %q, want fresh-session", got)
	}
}

func TestParseUnlockSessionRejectsEmptyOutput(t *testing.T) {
	_, err := parseUnlockSession([]byte(" \n\t"))
	if err == nil || !strings.Contains(err.Error(), "empty session") {
		t.Fatalf("parseUnlockSession() error = %v, want empty session", err)
	}
}

func TestEnsureUnlockedReturnsExportedSessionValidationError(t *testing.T) {
	resetBWInstalledCacheForTest()
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

func TestCreateFolder(t *testing.T) {
	t.Setenv("BW_SESSION", "test-session")
	logPath := filepath.Join(t.TempDir(), "bw.log")

	client := NewClient()
	client.execCommand = newTestBWExecCommand(t, "create_folder_ok", logPath)
	client.lookPath = func(string) (string, error) { return "/usr/local/bin/bw", nil }

	folder, err := client.CreateFolder("test-session", "prod")
	if err != nil {
		t.Fatalf("CreateFolder() error = %v", err)
	}
	if folder.ID != "folder-created" || folder.Name != "prod" {
		t.Fatalf("CreateFolder() = %+v, want created prod folder", folder)
	}

	calls := readTestBWCalls(t, logPath)
	if len(calls) != 1 {
		t.Fatalf("bw calls = %#v, want one create call", calls)
	}
	if !strings.Contains(calls[0], " create folder ") {
		t.Fatalf("bw call = %q, want create folder", calls[0])
	}
	if strings.Contains(calls[0], "--name") {
		t.Fatalf("bw call = %q, should use encoded JSON instead of --name", calls[0])
	}
}

func TestGetFolderID(t *testing.T) {
	tests := []struct {
		name      string
		scenario  string
		query     string
		wantID    string
		wantErr   bool
		errSubstr string
	}{
		{
			name:     "exact case-sensitive match",
			scenario: "folders_mixed_case",
			query:    "Dev",
			wantID:   "folder-dev-cap",
		},
		{
			name:     "case-insensitive fallback when exact missing",
			scenario: "folders_mixed_case",
			query:    "PROD",
			wantID:   "folder-prod-lower",
		},
		{
			name:     "exact match preferred over case-insensitive sibling",
			scenario: "folders_case_collision",
			query:    "Dev",
			wantID:   "folder-dev-cap",
		},
		{
			name:      "not found returns error",
			scenario:  "folders_mixed_case",
			query:     "missing",
			wantErr:   true,
			errSubstr: "folder 'missing' not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BW_SESSION", "test-session")
			logPath := filepath.Join(t.TempDir(), "bw.log")

			client := NewClient()
			client.execCommand = newTestBWExecCommand(t, tt.scenario, logPath)
			client.lookPath = func(string) (string, error) { return "/usr/local/bin/bw", nil }

			id, err := client.GetFolderID("test-session", tt.query)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("GetFolderID(%q) expected error, got id=%q", tt.query, id)
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("GetFolderID(%q) error = %v, want substring %q", tt.query, err, tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetFolderID(%q) unexpected error: %v", tt.query, err)
			}
			if id != tt.wantID {
				t.Fatalf("GetFolderID(%q) = %q, want %q", tt.query, id, tt.wantID)
			}
		})
	}
}

func newTestBWExecCommand(t *testing.T, scenario, logPath string) execCommandFunc {
	t.Helper()
	return func(name string, args ...string) *exec.Cmd {
		cmdArgs := append([]string{"-test.run=TestClientHelperProcess", "--", name}, args...)
		cmd := exec.Command(os.Args[0], cmdArgs...) //nolint:gosec // G204: test helper re-invokes the test binary with controlled args
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
		_, _ = fmt.Fprintf(os.Stderr, "unexpected command: %q\n", bwArgs[0])
		os.Exit(2)
	}

	logPath := os.Getenv("PKV_TEST_BW_LOG")
	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // G304: test opens a test-configured log path
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "open log: %v\n", err)
			os.Exit(2)
		}
		_, _ = fmt.Fprintf(f, "bw %s|env=%s\n", strings.Join(bwArgs[1:], " "), os.Getenv("BW_SESSION"))
		_ = f.Close()
	}

	joined := strings.Join(bwArgs[1:], " ")
	if joined == "--version" {
		switch os.Getenv("PKV_TEST_BW_SCENARIO") {
		case "version_command_fails":
			_, _ = fmt.Fprint(os.Stderr, "permission denied\n")
			os.Exit(1)
		case "version_malformed_output":
			_, _ = fmt.Fprint(os.Stdout, "Bitwarden CLI\n")
			os.Exit(0)
		default:
			_, _ = fmt.Fprint(os.Stdout, "2026.2.0\n")
			os.Exit(0)
		}
	}

	switch os.Getenv("PKV_TEST_BW_SCENARIO") {
	case "reuse_exported_session":
		if joined == "--nointeraction --session valid-session list folders" {
			_, _ = fmt.Fprint(os.Stdout, `[{"id":"folder-1","name":"dev"}]`)
			os.Exit(0)
		}
	case "refresh_expired_session":
		switch joined {
		case "--nointeraction --session expired-session list folders":
			_, _ = fmt.Fprint(os.Stderr, "Vault is locked.\n")
			os.Exit(1)
		case "--nointeraction status":
			_, _ = fmt.Fprint(os.Stdout, `{"status":"locked","userEmail":"dev@example.com"}`)
			os.Exit(0)
		case "--raw unlock":
			_, _ = fmt.Fprint(os.Stdout, "fresh-session\n")
			os.Exit(0)
		case "--nointeraction --session fresh-session list folders":
			_, _ = fmt.Fprint(os.Stdout, `[{"id":"folder-1","name":"dev"}]`)
			os.Exit(0)
		}
	case "exported_session_network_error":
		if joined == "--nointeraction --session flaky-session list folders" {
			_, _ = fmt.Fprint(os.Stderr, "network unreachable\n")
			os.Exit(1)
		}
	case "sync_ok":
		if strings.HasPrefix(joined, "--nointeraction --session ") && strings.HasSuffix(joined, " sync") {
			_, _ = fmt.Fprint(os.Stdout, "Syncing complete\n")
			os.Exit(0)
		}
	case "sync_create_ok":
		switch {
		case strings.HasPrefix(joined, "--nointeraction --session ") && strings.HasSuffix(joined, " sync"):
			_, _ = fmt.Fprint(os.Stdout, "Syncing complete\n")
			os.Exit(0)
		case strings.HasPrefix(joined, "--nointeraction --session ") && strings.Contains(joined, " create item "):
			_, _ = fmt.Fprint(os.Stdout, `{"id":"created-item"}`)
			os.Exit(0)
		}
	case "sync_edit_ok":
		switch {
		case strings.HasPrefix(joined, "--nointeraction --session ") && strings.HasSuffix(joined, " sync"):
			_, _ = fmt.Fprint(os.Stdout, "Syncing complete\n")
			os.Exit(0)
		case strings.HasPrefix(joined, "--nointeraction --session ") && strings.Contains(joined, " edit item item-1 "):
			_, _ = fmt.Fprint(os.Stdout, `{"success":true}`)
			os.Exit(0)
		}
	case "sync_delete_ok":
		switch {
		case strings.HasPrefix(joined, "--nointeraction --session ") && strings.HasSuffix(joined, " sync"):
			_, _ = fmt.Fprint(os.Stdout, "Syncing complete\n")
			os.Exit(0)
		case strings.HasPrefix(joined, "--nointeraction --session ") && strings.HasSuffix(joined, " delete item item-1"):
			_, _ = fmt.Fprint(os.Stdout, `{"success":true}`)
			os.Exit(0)
		}
	case "folders_mixed_case":
		// "Dev" exists exactly; "prod" exists only in lowercase.
		if strings.HasPrefix(joined, "--nointeraction --session test-session list folders --search ") {
			_, _ = fmt.Fprint(os.Stdout, `[{"id":"folder-dev-cap","name":"Dev"},{"id":"folder-prod-lower","name":"prod"}]`)
			os.Exit(0)
		}
	case "folders_case_collision":
		// Both "Dev" and "dev" exist; exact match must win regardless of order.
		if strings.HasPrefix(joined, "--nointeraction --session test-session list folders --search ") {
			_, _ = fmt.Fprint(os.Stdout, `[{"id":"folder-dev-lower","name":"dev"},{"id":"folder-dev-cap","name":"Dev"}]`)
			os.Exit(0)
		}
	case "create_folder_ok":
		if strings.HasPrefix(joined, "--nointeraction --session test-session create folder ") {
			parts := strings.Fields(joined)
			if len(parts) != 6 {
				_, _ = fmt.Fprintf(os.Stderr, "unexpected create folder args: %q\n", joined)
				os.Exit(2)
			}
			decoded, err := base64.StdEncoding.DecodeString(parts[5])
			if err != nil || string(decoded) != `{"name":"prod"}` {
				_, _ = fmt.Fprintf(os.Stderr, "invalid folder payload: %q err=%v\n", string(decoded), err)
				os.Exit(2)
			}
			_, _ = fmt.Fprint(os.Stdout, `{"id":"folder-created","name":"prod"}`)
			os.Exit(0)
		}
	}

	_, _ = fmt.Fprintf(os.Stderr, "unexpected bw args for %s: %q\n", os.Getenv("PKV_TEST_BW_SCENARIO"), joined)
	os.Exit(2)
}
