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
