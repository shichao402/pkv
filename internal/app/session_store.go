package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const sessionFileName = ".pkv/session"

// SessionFilePath returns the path to the persisted Bitwarden session file.
func SessionFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, sessionFileName), nil
}

// ReadSession reads a persisted session from ~/.pkv/session.
// Returns ("", nil) when the file does not exist.
func ReadSession() (string, error) {
	path, err := SessionFilePath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// WriteSession persists session to ~/.pkv/session with mode 0600.
func WriteSession(session string) error {
	session = strings.TrimSpace(session)
	if session == "" {
		return fmt.Errorf("session is empty")
	}
	path, err := SessionFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(session+"\n"), 0o600)
}
