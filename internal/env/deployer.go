package env

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/shichao402/pkv/internal/bw/types"
	"github.com/shichao402/pkv/internal/state"
)

type EnvVar struct {
	Key   string
	Value string
}

// ConfirmFunc is called to ask the user whether to overwrite conflicting keys.
// It receives the list of conflicts and returns true if the user wants to overwrite.
type ConfirmFunc func(conflicts []ConflictInfo) (bool, error)

type Deployer struct {
	state   *state.State
	confirm ConfirmFunc
}

func NewDeployer(st *state.State, confirm ConfirmFunc) *Deployer {
	return &Deployer{state: st, confirm: confirm}
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

		// Handle quoted values with proper validation
		value = stripQuotes(value, lineNum)

		vars = append(vars, EnvVar{Key: key, Value: value})
	}
	return vars, scanner.Err()
}

// stripQuotes removes matching quotes from the value if present.
// Returns the unquoted value, or the original value if quotes don't match.
func stripQuotes(value string, lineNum int) string {
	if len(value) < 2 {
		return value
	}

	// Check for matching quotes
	first := value[0]
	last := value[len(value)-1]

	if first == '"' && last == '"' {
		return value[1 : len(value)-1]
	}
	if first == '\'' && last == '\'' {
		return value[1 : len(value)-1]
	}

	// No matching quotes, return as-is
	// (including cases like "value' or value" with unmatched quotes)
	return value
}

// Deploy parses a Secure Note as KEY=VALUE pairs and sets them as persistent environment variables.
// It detects key conflicts with other deployed folders and asks the user for confirmation.
// A snapshot of all deployed key-value pairs is saved for later restoration on clean.
func (d *Deployer) Deploy(item types.Item) ([]EnvVar, error) {
	if item.Notes == "" {
		return nil, fmt.Errorf("item '%s' has no content", item.Name)
	}

	vars, err := ParseEnvVars(item.Notes)
	if err != nil {
		return nil, fmt.Errorf("parse '%s': %w", item.Name, err)
	}

	// Check for conflicts with other deployed folders
	conflicts, err := DetectConflicts(vars, item.ID)
	if err != nil {
		return nil, fmt.Errorf("conflict detection: %w", err)
	}

	// Build set of keys to skip (user declined overwrite)
	skipKeys := make(map[string]bool)
	if len(conflicts) > 0 {
		overwrite, err := d.confirm(conflicts)
		if err != nil {
			return nil, fmt.Errorf("confirm overwrite: %w", err)
		}
		if !overwrite {
			for _, c := range conflicts {
				skipKeys[c.Key] = true
			}
		}
	}

	var failedVars []string
	var successVars []EnvVar
	for _, v := range vars {
		if skipKeys[v.Key] {
			fmt.Printf("    - %s (skipped, owned by '%s')\n", v.Key, conflictOwner(conflicts, v.Key))
			continue
		}
		if err := setPersistentEnv(v.Key, v.Value); err != nil {
			failedVars = append(failedVars, fmt.Sprintf("%s (%v)", v.Key, err))
			continue
		}
		_ = os.Setenv(v.Key, v.Value)
		successVars = append(successVars, v)
	}

	// Save snapshot with all successfully set vars (for restoration on clean)
	snapVars := make(map[string]string, len(successVars))
	for _, v := range successVars {
		snapVars[v.Key] = v.Value
	}
	if len(snapVars) > 0 {
		if err := SaveSnapshot(Snapshot{
			ItemID: item.ID,
			Name:   item.Name,
			Vars:   snapVars,
		}); err != nil {
			return successVars, fmt.Errorf("save snapshot: %w", err)
		}
	}

	// Record successfully set variables in state
	keys := make([]string, len(successVars))
	for i, v := range successVars {
		keys[i] = v.Key
	}
	if len(keys) > 0 {
		d.state.AddEnv(state.EnvEntry{
			ItemID: item.ID,
			Name:   item.Name,
			Keys:   keys,
		})
	}

	// Report all failures after attempting all variables
	if len(failedVars) > 0 {
		if len(successVars) > 0 {
			return successVars, fmt.Errorf("partial failure: %s (deployed %d/%d variables)", strings.Join(failedVars, "; "), len(successVars), len(vars))
		}
		return nil, fmt.Errorf("all %d variables failed: %s", len(vars), strings.Join(failedVars, "; "))
	}

	return successVars, nil
}

// Remove unsets previously deployed environment variables.
// For each key, it checks whether another deployed folder also has the same key.
// If so, the value is restored from that folder's snapshot instead of being deleted.
func (d *Deployer) Remove(entry state.EnvEntry) error {
	orderedIDs := d.state.EnvItemIDsByRecency()

	var failedKeys []string
	for _, key := range entry.Keys {
		// Check if another folder's snapshot has a value to restore
		restoredVal, restoredFrom, found := FindRestorationValue(key, entry.ItemID, orderedIDs)
		if found {
			// Restore the value from the other folder
			if err := setPersistentEnv(key, restoredVal); err != nil {
				failedKeys = append(failedKeys, fmt.Sprintf("%s (%v)", key, err))
				continue
			}
			_ = os.Setenv(key, restoredVal)
			fmt.Printf("    ~ %s (restored from '%s')\n", key, restoredFrom)
		} else {
			// No other folder uses this key, remove it
			if err := removePersistentEnv(key); err != nil {
				failedKeys = append(failedKeys, fmt.Sprintf("%s (%v)", key, err))
				continue
			}
			_ = os.Unsetenv(key)
		}
	}

	// Delete the snapshot file for this item
	if err := DeleteSnapshot(entry.ItemID); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: failed to delete snapshot for '%s': %v\n", entry.Name, err)
	}

	if len(failedKeys) > 0 {
		if len(failedKeys) == len(entry.Keys) {
			return fmt.Errorf("failed to remove all %d variables: %s", len(entry.Keys), strings.Join(failedKeys, "; "))
		}
		return fmt.Errorf("partial failure removing variables: %s (removed %d/%d)", strings.Join(failedKeys, "; "), len(entry.Keys)-len(failedKeys), len(entry.Keys))
	}
	return nil
}

// conflictOwner returns the owner name for a conflicting key.
func conflictOwner(conflicts []ConflictInfo, key string) string {
	for _, c := range conflicts {
		if c.Key == key {
			return c.ExistingName
		}
	}
	return "unknown"
}
