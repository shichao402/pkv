package env

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shichao402/pkv/internal/bw/types"
	"github.com/shichao402/pkv/internal/state"
)

const envArtifactDir = ".pkv/env"

type EnvVar struct {
	Key   string
	Value string
}

type Deployer struct {
	state *state.State
}

func NewDeployer(st *state.State) *Deployer {
	return &Deployer{state: st}
}

// ParseEnvVars parses KEY=VALUE lines from note content.
// Supports: KEY=VALUE, export KEY=VALUE, # comments, empty lines, quoted values.
func ParseEnvVars(content string) ([]EnvVar, error) {
	var vars []EnvVar
	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		line = strings.TrimSpace(line)

		idx := strings.Index(line, "=")
		if idx < 1 {
			return nil, fmt.Errorf("line %d: invalid format (expected KEY=VALUE): %s", lineNum, line)
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		value = stripQuotes(value, lineNum)

		if !validEnvVarName(key) {
			return nil, fmt.Errorf("line %d: invalid environment variable name: %s", lineNum, key)
		}

		vars = append(vars, EnvVar{Key: key, Value: value})
	}
	return vars, scanner.Err()
}

func stripQuotes(value string, _ int) string {
	if len(value) < 2 {
		return value
	}
	first := value[0]
	last := value[len(value)-1]
	if first == '"' && last == '"' {
		return value[1 : len(value)-1]
	}
	if first == '\'' && last == '\'' {
		return value[1 : len(value)-1]
	}
	return value
}

// Deploy writes folder-scoped env artifacts for the managed env note.
// It does not mutate the process or system environment.
func (d *Deployer) Deploy(folder string, item types.Item) (state.EnvEntry, error) {
	vars, err := ParseEnvVars(item.Notes)
	if err != nil {
		return state.EnvEntry{}, fmt.Errorf("parse '%s': %w", item.Name, err)
	}

	jsonPath, shellPath, powerShellPath, err := artifactPaths(folder)
	if err != nil {
		return state.EnvEntry{}, err
	}

	if err := os.MkdirAll(filepath.Dir(jsonPath), 0o700); err != nil {
		return state.EnvEntry{}, err
	}

	sorted := append([]EnvVar(nil), vars...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Key < sorted[j].Key
	})

	if err := writeJSONArtifact(jsonPath, sorted); err != nil {
		return state.EnvEntry{}, err
	}
	if err := writeShellArtifact(shellPath, folder, sorted); err != nil {
		return state.EnvEntry{}, err
	}
	if err := writePowerShellArtifact(powerShellPath, folder, sorted); err != nil {
		return state.EnvEntry{}, err
	}

	keys := make([]string, len(sorted))
	for i, v := range sorted {
		keys[i] = v.Key
	}

	entry := state.EnvEntry{
		ItemID:         item.ID,
		Folder:         folder,
		Name:           item.Name,
		Keys:           keys,
		JSONPath:       jsonPath,
		ShellPath:      shellPath,
		PowerShellPath: powerShellPath,
	}
	d.state.AddEnv(entry)
	return entry, nil
}

// Remove removes local env artifacts for a folder-scoped env note.
func (d *Deployer) Remove(entry state.EnvEntry) error {
	jsonPath, shellPath, powerShellPath, err := artifactPaths(entryFolder(entry))
	if err != nil {
		return err
	}
	if entry.JSONPath != "" {
		jsonPath = entry.JSONPath
	}
	if entry.ShellPath != "" {
		shellPath = entry.ShellPath
	}
	if entry.PowerShellPath != "" {
		powerShellPath = entry.PowerShellPath
	}

	var errs []string
	for _, path := range []string{jsonPath, shellPath, powerShellPath} {
		if path == "" {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("remove %s: %v", path, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func entryFolder(entry state.EnvEntry) string {
	if entry.Folder != "" {
		return entry.Folder
	}
	return entry.Name
}

func artifactPaths(folder string) (jsonPath, shellPath, powerShellPath string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", "", err
	}
	base := sanitizeFolderName(folder)
	dir := filepath.Join(home, envArtifactDir)
	return filepath.Join(dir, base+".json"), filepath.Join(dir, base+".sh"), filepath.Join(dir, base+".ps1"), nil
}

func sanitizeFolderName(name string) string {
	if name == "" {
		return "default"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		case r == ' ' || r == '/' || r == '\\':
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "default"
	}
	return b.String()
}

func writeJSONArtifact(path string, vars []EnvVar) error {
	data := make(map[string]string, len(vars))
	for _, v := range vars {
		data[v.Key] = v.Value
	}
	body, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json artifact: %w", err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return fmt.Errorf("write json artifact: %w", err)
	}
	return nil
}

func writeShellArtifact(path, folder string, vars []EnvVar) error {
	var lines []string
	lines = append(lines, fmt.Sprintf("# PKV env for folder %q", folder))
	for _, v := range vars {
		lines = append(lines, fmt.Sprintf("export %s='%s'", v.Key, escapeShellValue(v.Value)))
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return fmt.Errorf("write shell artifact: %w", err)
	}
	return nil
}

func writePowerShellArtifact(path, folder string, vars []EnvVar) error {
	var lines []string
	lines = append(lines, fmt.Sprintf("# PKV env for folder %q", folder))
	for _, v := range vars {
		lines = append(lines, fmt.Sprintf("$env:%s = '%s'", v.Key, escapePowerShellValue(v.Value)))
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return fmt.Errorf("write powershell artifact: %w", err)
	}
	return nil
}

func escapeShellValue(s string) string {
	return strings.ReplaceAll(s, "'", `'\''`)
}

func escapePowerShellValue(s string) string {
	s = strings.ReplaceAll(s, "'", "''")
	s = strings.ReplaceAll(s, "`", "``")
	s = strings.ReplaceAll(s, "$", "`$")
	return s
}

func validEnvVarName(name string) bool {
	if name == "" {
		return false
	}
	c := name[0]
	if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_') {
		return false
	}
	for i := 1; i < len(name); i++ {
		c = name[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}
