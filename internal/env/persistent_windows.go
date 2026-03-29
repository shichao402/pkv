//go:build windows

package env

import (
	"fmt"
	"os/exec"
	"strings"
)

func setPersistentEnv(key, value string) error {
	script := fmt.Sprintf("[Environment]::SetEnvironmentVariable('%s', '%s', 'User')",
		escapePS(key), escapePS(value))
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func removePersistentEnv(key string) error {
	script := fmt.Sprintf("[Environment]::SetEnvironmentVariable('%s', $null, 'User')",
		escapePS(key))
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func escapePS(s string) string {
	// Escape single quotes by doubling them (PowerShell escaping)
	s = strings.ReplaceAll(s, "'", "''")
	// Escape backticks (PowerShell escape/command substitution character)
	s = strings.ReplaceAll(s, "`", "``")
	// Escape dollar signs (PowerShell variable expansion character)
	s = strings.ReplaceAll(s, "$", "`$")
	return s
}
