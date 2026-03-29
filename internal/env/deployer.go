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

		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		vars = append(vars, EnvVar{Key: key, Value: value})
	}
	return vars, scanner.Err()
}

// Deploy parses a Secure Note as KEY=VALUE pairs and sets them as persistent environment variables.
func (d *Deployer) Deploy(item types.Item) ([]EnvVar, error) {
	if item.Notes == "" {
		return nil, fmt.Errorf("item '%s' has no content", item.Name)
	}

	vars, err := ParseEnvVars(item.Notes)
	if err != nil {
		return nil, fmt.Errorf("parse '%s': %w", item.Name, err)
	}

	for _, v := range vars {
		if err := setPersistentEnv(v.Key, v.Value); err != nil {
			return nil, fmt.Errorf("set %s: %w", v.Key, err)
		}
		os.Setenv(v.Key, v.Value)
	}

	keys := make([]string, len(vars))
	for i, v := range vars {
		keys[i] = v.Key
	}
	d.state.AddEnv(state.EnvEntry{
		ItemID: item.ID,
		Name:   item.Name,
		Keys:   keys,
	})

	return vars, nil
}

// Remove unsets previously deployed environment variables.
func (d *Deployer) Remove(entry state.EnvEntry) error {
	for _, key := range entry.Keys {
		if err := removePersistentEnv(key); err != nil {
			return fmt.Errorf("remove %s: %w", key, err)
		}
		os.Unsetenv(key)
	}
	return nil
}
