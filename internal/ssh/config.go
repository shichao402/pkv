package ssh

import (
	"fmt"
	"os"
	"strings"
)

const (
	markerStart = "# >>> PKV MANAGED START: %s <<<"
	markerEnd   = "# >>> PKV MANAGED END: %s <<<"
)

// AddHostEntries appends a managed block of Host entries to the SSH config file.
// Each host gets a full config block with HostName, IdentityFile, IdentitiesOnly,
// and StrictHostKeyChecking no (since known_hosts is managed separately by PKV).
func AddHostEntries(configPath, keyName, keyFile string, hosts []string) error {
	existing, err := readFileOrEmpty(configPath)
	if err != nil {
		return err
	}

	// Remove any existing block for this key first
	existing = removeManagedBlock(existing, keyName)

	// Build new block
	var block strings.Builder
	block.WriteString("\n")
	fmt.Fprintf(&block, markerStart, keyName)
	block.WriteString("\n")
	for _, host := range hosts {
		hostname, port := parseHostPort(host)
		fmt.Fprintf(&block, "Host %s\n", hostname)
		fmt.Fprintf(&block, "    HostName %s\n", hostname)
		if port != "" {
			fmt.Fprintf(&block, "    Port %s\n", port)
		}
		fmt.Fprintf(&block, "    IdentityFile %s\n", keyFile)
		block.WriteString("    IdentitiesOnly yes\n")
		block.WriteString("\n")
	}
	fmt.Fprintf(&block, markerEnd, keyName)
	block.WriteString("\n")

	content := strings.TrimRight(existing, "\n") + block.String()
	return os.WriteFile(configPath, []byte(content), 0o600)
}

// RemoveHostEntries removes the managed block for the given key name from the SSH config.
func RemoveHostEntries(configPath, keyName string) error {
	existing, err := readFileOrEmpty(configPath)
	if err != nil {
		return err
	}

	if existing == "" {
		return nil
	}

	cleaned := removeManagedBlock(existing, keyName)
	cleaned = collapseBlankLines(cleaned)
	return os.WriteFile(configPath, []byte(cleaned), 0o600)
}

func removeManagedBlock(content, keyName string) string {
	startMarker := fmt.Sprintf(markerStart, keyName)
	endMarker := fmt.Sprintf(markerEnd, keyName)

	lines := strings.Split(content, "\n")
	var result []string
	inBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == startMarker {
			inBlock = true
			continue
		}
		if trimmed == endMarker {
			inBlock = false
			continue
		}
		if !inBlock {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// parseHostPort splits "host:port" into (host, port). If no port, returns (host, "").
func parseHostPort(s string) (host, port string) {
	// Handle [ipv6]:port
	if strings.HasPrefix(s, "[") {
		if idx := strings.LastIndex(s, "]:"); idx != -1 {
			return s[:idx+1], s[idx+2:]
		}
		return s, ""
	}
	// Handle host:port (only if exactly one colon, to avoid confusing with ipv6)
	if strings.Count(s, ":") == 1 {
		parts := strings.SplitN(s, ":", 2)
		return parts[0], parts[1]
	}
	return s, ""
}

func readFileOrEmpty(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

func collapseBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	var result []string
	prevBlank := false
	for _, line := range lines {
		blank := strings.TrimSpace(line) == ""
		if blank && prevBlank {
			continue
		}
		result = append(result, line)
		prevBlank = blank
	}
	return strings.Join(result, "\n")
}
