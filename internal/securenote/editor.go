package securenote

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// ResolveEditor returns the editor command used for text content.
func ResolveEditor() string {
	return resolveEditor(os.Getenv, exec.LookPath, runtime.GOOS)
}

func resolveEditor(getenv func(string) string, lookPath func(string) (string, error), goos string) string {
	if editor := strings.TrimSpace(getenv("VISUAL")); editor != "" {
		return editor
	}
	if editor := strings.TrimSpace(getenv("EDITOR")); editor != "" {
		return editor
	}
	if goos == "windows" {
		return "notepad"
	}
	if _, err := lookPath("vim"); err == nil {
		return "vim"
	}
	return "vi"
}

// OpenEditor opens the user's preferred editor with initialContent,
// waits for editing to complete, and returns the edited content.
func OpenEditor(initialContent string) (string, error) {
	editor := ResolveEditor()

	tmpFile, err := os.CreateTemp("", "pkv-*.txt")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmpFile.WriteString(initialContent); err != nil {
		_ = tmpFile.Close()
		return "", fmt.Errorf("write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}

	// Split editor command in case it contains args (e.g. "code --wait")
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return "", fmt.Errorf("editor command is empty")
	}
	args := make([]string, 0, len(parts))
	args = append(args, parts[1:]...)
	args = append(args, tmpPath)
	cmd := exec.Command(parts[0], args...) //nolint:gosec // G204: user-provided editor binary is intentional
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("editor exited with error: %w", err)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf("read edited file: %w", err)
	}

	return string(data), nil
}
