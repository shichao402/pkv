package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndReadSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := WriteSession("test-session-token"); err != nil {
		t.Fatalf("WriteSession() error = %v", err)
	}

	path, err := SessionFilePath()
	if err != nil {
		t.Fatalf("SessionFilePath() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("session file mode = %o, want 0600", info.Mode().Perm())
	}

	session, err := ReadSession()
	if err != nil {
		t.Fatalf("ReadSession() error = %v", err)
	}
	if session != "test-session-token" {
		t.Fatalf("ReadSession() = %q, want test-session-token", session)
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("Stat(dir) error = %v", err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("session dir mode = %o, want 0700", dirInfo.Mode().Perm())
	}
}

func TestReadSessionMissingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	session, err := ReadSession()
	if err != nil {
		t.Fatalf("ReadSession() error = %v, want nil", err)
	}
	if session != "" {
		t.Fatalf("ReadSession() = %q, want empty", session)
	}
}
