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
	"github.com/shichao402/pkv/internal/diag"
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
		diag.Printf("failed to parse env note for folder %q item %q (%s): %v", folder, item.Name, item.ID, err)
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

	diag.Printf("deploying env artifacts for folder %q item %q (%s): %d vars", folder, item.Name, item.ID, len(sorted))
	diag.Printf("env artifact targets: json=%q shell=%q powershell=%q", jsonPath, shellPath, powerShellPath)

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
	diag.Printf("env artifacts written for folder %q", folder)
	return entry, nil
}

// DeployMerged writes env artifacts from a merge result produced by
// MergePkvEnvNotes. The artifacts always land under the current folder (the
// first element of result.Chain), regardless of which folder actually
// supplied each variable.
//
// This is the include-aware path used by `pkv get <folder> env`. It differs
// from Deploy in two ways:
//
//   - It does not parse a single item; vars/sources are already resolved.
//   - The state.EnvEntry it records has no ItemID when the current folder
//     itself has no pkv.env note. AddEnv falls back to Folder-based dedup in
//     that case so the record still updates cleanly on re-run.
//
// currentItem is the `pkv.env` item from chain[0] if present; pass the zero
// types.Item (hasCurrent=false) to signal that chain[0] has no env note.
//
// SourceFolder attribution:
//
//   - hasCurrent=true: chain[0] owns its own pkv.env, so the state record's
//     SourceFolder stays empty (the record and the folder it indexes are the
//     same). Per-key overrides from deeper folders are visible in
//     result.Conflicts but not recorded per-var in state (EnvEntry is
//     folder-scoped, not key-scoped).
//   - hasCurrent=false: chain[0] is a thin project; env comes entirely from
//     includes. SourceFolder is set to the first folder on the chain that
//     actually contributed a value (deterministic via chain order).
//
// The caller is responsible for surfacing result.Conflicts to the user; this
// function does not log them.
func (d *Deployer) DeployMerged(folder string, result MergeResult, currentItem types.Item, hasCurrent bool) (state.EnvEntry, error) {
	jsonPath, shellPath, powerShellPath, err := artifactPaths(folder)
	if err != nil {
		return state.EnvEntry{}, err
	}

	if err := os.MkdirAll(filepath.Dir(jsonPath), 0o700); err != nil {
		return state.EnvEntry{}, err
	}

	// result.Vars is already sorted by key (see merge.go), so feed the shared
	// writers directly without re-sorting.
	sorted := make([]EnvVar, len(result.Vars))
	for i, v := range result.Vars {
		sorted[i] = v.EnvVar
	}

	diag.Printf("deploying merged env artifacts for folder %q: chain=%v vars=%d conflicts=%d",
		folder, result.Chain, len(sorted), len(result.Conflicts))
	diag.Printf("env artifact targets: json=%q shell=%q powershell=%q", jsonPath, shellPath, powerShellPath)

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
		Folder:         folder,
		Keys:           keys,
		JSONPath:       jsonPath,
		ShellPath:      shellPath,
		PowerShellPath: powerShellPath,
	}
	if hasCurrent {
		entry.ItemID = currentItem.ID
		entry.Name = currentItem.Name
		// SourceFolder stays empty: the record belongs to chain[0] itself.
	} else {
		// No local pkv.env; the record still needs a stable name for list/clean
		// UX. Use the reserved name as a placeholder so the existing Remove /
		// FindEnvsByFolder code paths read cleanly.
		entry.Name = types.ReservedEnvNoteName
		// Record which folder actually supplied the env payload so list/clean
		// UX can surface the attribution. Pick the first chain folder that
		// appears as a Source among the merged vars; chain order is
		// authoritative (chain[0] is the current folder, which by definition
		// contributed nothing here).
		entry.SourceFolder = firstContributingSource(result)
	}
	d.state.AddEnv(entry)
	diag.Printf("merged env artifacts written for folder %q (chain=%v)", folder, result.Chain)
	return entry, nil
}

// firstContributingSource returns the first folder on result.Chain that
// appears as a Source in result.Vars. Used to attribute thin-project env
// records (where chain[0] has no pkv.env of its own). Returns empty string
// only if result.Vars is empty, which shouldn't happen because DeployMerged
// is only reached when notesByFolder is non-empty.
func firstContributingSource(result MergeResult) string {
	contributors := make(map[string]bool, len(result.Vars))
	for _, v := range result.Vars {
		contributors[v.Source] = true
	}
	for _, folder := range result.Chain {
		if contributors[folder] {
			return folder
		}
	}
	return ""
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

	diag.Printf("removing env artifacts for folder %q: json=%q shell=%q powershell=%q", entryFolder(entry), jsonPath, shellPath, powerShellPath)
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
		diag.Printf("failed to remove env artifacts for folder %q: %s", entryFolder(entry), strings.Join(errs, "; "))
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
	if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && c != '_' {
		return false
	}
	for i := 1; i < len(name); i++ {
		c = name[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}
