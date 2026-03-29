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
	return strings.ReplaceAll(s, "'", "''")
}
