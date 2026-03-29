//go:build !windows

package env

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	envFileName = ".pkv/env.sh"
	sourceLine  = `[ -f "$HOME/.pkv/env.sh" ] && . "$HOME/.pkv/env.sh"`
	sourceTag   = ".pkv/env.sh"
)

func setPersistentEnv(key, value string) error {
	envFile, err := envFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(envFile), 0700); err != nil {
		return err
	}

	lines, _ := readLines(envFile)

	exportLine := fmt.Sprintf("export %s='%s'", key, escapeShellQuote(value))
	prefix := fmt.Sprintf("export %s=", key)

	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			lines[i] = exportLine
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, exportLine)
	}

	if err := os.WriteFile(envFile, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		return err
	}
	return ensureSourced()
}

func removePersistentEnv(key string) error {
	envFile, err := envFilePath()
	if err != nil {
		return err
	}

	lines, err := readLines(envFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	prefix := fmt.Sprintf("export %s=", key)
	var filtered []string
	for _, line := range lines {
		if !strings.HasPrefix(line, prefix) {
			filtered = append(filtered, line)
		}
	}

	if len(filtered) == 0 {
		return os.Remove(envFile)
	}
	return os.WriteFile(envFile, []byte(strings.Join(filtered, "\n")+"\n"), 0600)
}

func envFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, envFileName), nil
}

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := strings.TrimRight(string(data), "\n")
	if content == "" {
		return nil, nil
	}
	return strings.Split(content, "\n"), nil
}

func ensureSourced() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	for _, rc := range []string{".bashrc", ".zshrc"} {
		rcPath := filepath.Join(home, rc)
		if _, err := os.Stat(rcPath); os.IsNotExist(err) {
			continue
		}
		data, err := os.ReadFile(rcPath)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), sourceTag) {
			continue
		}
		f, err := os.OpenFile(rcPath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			continue
		}
		fmt.Fprintf(f, "\n# PKV managed environment variables\n%s\n", sourceLine)
		f.Close()
	}
	return nil
}

func escapeShellQuote(s string) string {
	return strings.ReplaceAll(s, "'", `'\''`)
}
