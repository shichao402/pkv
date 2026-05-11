package ssh

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func setHome(t *testing.T, dir string) {
	t.Helper()
	// os.UserHomeDir on darwin/linux uses HOME; on windows it consults
	// USERPROFILE. We don't run tests on Windows for this package, but be
	// defensive anyway.
	t.Setenv("HOME", dir)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", dir)
	}
}

const samplePub = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleExampleExampleExampleExampleExample alice@host"

func TestAppendAuthorizedKey_CreatesFileWith0600(t *testing.T) {
	tmp := t.TempDir()
	setHome(t, tmp)

	added, path, err := AppendAuthorizedKey(samplePub)
	if err != nil {
		t.Fatalf("AppendAuthorizedKey: %v", err)
	}
	if !added {
		t.Fatal("expected added=true on first append")
	}
	if path != filepath.Join(tmp, ".ssh", "authorized_keys") {
		t.Fatalf("unexpected path: %s", path)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("authorized_keys perm = %o, want 0600", info.Mode().Perm())
	}
	sshInfo, err := os.Stat(filepath.Join(tmp, ".ssh"))
	if err != nil {
		t.Fatalf("stat .ssh: %v", err)
	}
	if sshInfo.Mode().Perm() != 0o700 {
		t.Fatalf(".ssh perm = %o, want 0700", sshInfo.Mode().Perm())
	}
	got, _ := os.ReadFile(path)
	if !strings.HasSuffix(string(got), "\n") {
		t.Fatalf("file should end with newline, got %q", got)
	}
}

func TestAppendAuthorizedKey_SkipsDuplicate(t *testing.T) {
	tmp := t.TempDir()
	setHome(t, tmp)

	if _, _, err := AppendAuthorizedKey(samplePub); err != nil {
		t.Fatalf("first append: %v", err)
	}
	// Re-append with a different comment; should be detected as duplicate.
	dup := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleExampleExampleExampleExampleExample bob@other"
	added, _, err := AppendAuthorizedKey(dup)
	if err != nil {
		t.Fatalf("dup append: %v", err)
	}
	if added {
		t.Fatal("expected added=false for duplicate key material")
	}

	content, _ := os.ReadFile(filepath.Join(tmp, ".ssh", "authorized_keys"))
	if strings.Count(string(content), "ssh-ed25519 ") != 1 {
		t.Fatalf("expected exactly one ed25519 line, got: %q", content)
	}
}

func TestAppendAuthorizedKey_DifferentMaterialAppends(t *testing.T) {
	tmp := t.TempDir()
	setHome(t, tmp)

	if _, _, err := AppendAuthorizedKey(samplePub); err != nil {
		t.Fatalf("first append: %v", err)
	}
	other := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDifferentMaterialDifferentMaterialDifferentX alice@host"
	added, _, err := AppendAuthorizedKey(other)
	if err != nil {
		t.Fatalf("second append: %v", err)
	}
	if !added {
		t.Fatal("expected added=true for different key material")
	}
	content, _ := os.ReadFile(filepath.Join(tmp, ".ssh", "authorized_keys"))
	if strings.Count(string(content), "ssh-ed25519 ") != 2 {
		t.Fatalf("expected two ed25519 lines, got: %q", content)
	}
}

func TestAppendAuthorizedKey_AppendsNewlineWhenMissing(t *testing.T) {
	tmp := t.TempDir()
	setHome(t, tmp)

	sshDir := filepath.Join(tmp, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatalf("mkdir .ssh: %v", err)
	}
	path := filepath.Join(sshDir, "authorized_keys")
	// Pre-existing file without trailing newline.
	if err := os.WriteFile(path, []byte("ssh-rsa AAAAprior existing@host"), 0o600); err != nil {
		t.Fatalf("write pre-existing: %v", err)
	}

	added, _, err := AppendAuthorizedKey(samplePub)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if !added {
		t.Fatal("expected added=true")
	}
	got, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(got), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), got)
	}
}

func TestAppendAuthorizedKey_RejectsEmptyAndMalformed(t *testing.T) {
	tmp := t.TempDir()
	setHome(t, tmp)

	if _, _, err := AppendAuthorizedKey(""); err == nil {
		t.Fatal("expected error for empty key")
	}
	if _, _, err := AppendAuthorizedKey("ssh-ed25519"); err == nil {
		t.Fatal("expected error for missing material")
	}
}
