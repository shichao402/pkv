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
// If some variables fail to set, it will try to continue with others and report partial failures.
func (d *Deployer) Deploy(item types.Item) ([]EnvVar, error) {
	if item.Notes == "" {
		return nil, fmt.Errorf("item '%s' has no content", item.Name)
	}

	vars, err := ParseEnvVars(item.Notes)
	if err != nil {
		return nil, fmt.Errorf("parse '%s': %w", item.Name, err)
	}

	var failedVars []string
	var successVars []EnvVar
	for _, v := range vars {
		if err := setPersistentEnv(v.Key, v.Value); err != nil {
			failedVars = append(failedVars, fmt.Sprintf("%s (%v)", v.Key, err))
			continue
		}
		_ = os.Setenv(v.Key, v.Value)
		successVars = append(successVars, v)
	}

	// Only record successfully set variables
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
// Attempts to remove all variables and reports any failures at the end.
func (d *Deployer) Remove(entry state.EnvEntry) error {
	var failedKeys []string
	for _, key := range entry.Keys {
		if err := removePersistentEnv(key); err != nil {
			failedKeys = append(failedKeys, fmt.Sprintf("%s (%v)", key, err))
			continue
		}
		_ = os.Unsetenv(key)
	}

	if len(failedKeys) > 0 {
		if len(failedKeys) == len(entry.Keys) {
			return fmt.Errorf("failed to remove all %d variables: %s", len(entry.Keys), strings.Join(failedKeys, "; "))
		}
		return fmt.Errorf("partial failure removing variables: %s (removed %d/%d)", strings.Join(failedKeys, "; "), len(entry.Keys)-len(failedKeys), len(entry.Keys))
	}
	return nil
}
