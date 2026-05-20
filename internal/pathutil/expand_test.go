package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "expand tilde path",
			path: "~/test.txt",
			want: filepath.Join(home, "test.txt"),
		},
		{
			name: "absolute path unchanged",
			path: "/tmp/test.txt",
			want: "/tmp/test.txt",
		},
		{
			name: "relative path unchanged",
			path: "test.txt",
			want: "test.txt",
		},
		{
			name: "empty path unchanged",
			path: "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandTilde(tt.path)
			if err != nil {
				t.Fatalf("ExpandTilde() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ExpandTilde() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRelativeFileNoteName(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)

	nested := filepath.Join(cwd, "xxx", "aaa", "bbb")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	file := filepath.Join(nested, "test.json")
	if err := os.WriteFile(file, []byte("{}"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "relative nested path",
			path: filepath.Join("xxx", "aaa", "bbb", "test.json"),
			want: "xxx/aaa/bbb/test.json",
		},
		{
			name: "absolute nested path",
			path: file,
			want: "xxx/aaa/bbb/test.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RelativeFileNoteName(tt.path)
			if err != nil {
				t.Fatalf("RelativeFileNoteName() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("RelativeFileNoteName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRelativeFileNoteNameRejectsOutsideCWD(t *testing.T) {
	cwd := t.TempDir()
	outside := t.TempDir()
	t.Chdir(cwd)

	file := filepath.Join(outside, "test.json")
	if err := os.WriteFile(file, []byte("{}"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := RelativeFileNoteName(file); err == nil {
		t.Fatal("RelativeFileNoteName() error = nil, want outside cwd error")
	}
}

func TestRelativeFileNoteNameRejectsSymlinkOutsideCWD(t *testing.T) {
	cwd := t.TempDir()
	outside := t.TempDir()
	t.Chdir(cwd)

	file := filepath.Join(outside, "test.json")
	if err := os.WriteFile(file, []byte("{}"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	link := filepath.Join(cwd, "linked.json")
	if err := os.Symlink(file, link); err != nil {
		t.Skipf("Symlink() unsupported: %v", err)
	}

	if _, err := RelativeFileNoteName("linked.json"); err == nil {
		t.Fatal("RelativeFileNoteName() error = nil, want symlink target outside cwd error")
	}
}
