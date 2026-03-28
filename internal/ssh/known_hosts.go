package ssh

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const knownHostsMarkerStart = "# >>> PKV MANAGED START <<<"
const knownHostsMarkerEnd = "# >>> PKV MANAGED END <<<"

// ScanAndAddKnownHosts runs ssh-keyscan for each host and appends results
// into ~/.ssh/known_hosts inside a managed marker block.
func ScanAndAddKnownHosts(sshDir string, hosts []string) error {
	if len(hosts) == 0 {
		return nil
	}

	knownHostsPath := filepath.Join(sshDir, "known_hosts")

	// Deduplicate hosts and extract hostname (strip port)
	scanTargets := make([]string, 0, len(hosts))
	seen := map[string]bool{}
	for _, h := range hosts {
		hostname, port := parseHostPort(h)
		var target string
		if port != "" && port != "22" {
			target = fmt.Sprintf("[%s]:%s", hostname, port)
		} else {
			target = hostname
		}
		if !seen[target] {
			seen[target] = true
			scanTargets = append(scanTargets, target)
		}
	}

	// Run ssh-keyscan
	var scannedKeys []string
	for _, target := range scanTargets {
		out, err := exec.Command("ssh-keyscan", "-T", "5", target).Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: ssh-keyscan failed for '%s': %v\n", target, err)
			continue
		}
		lines := strings.TrimSpace(string(out))
		if lines != "" {
			scannedKeys = append(scannedKeys, lines)
		}
	}

	if len(scannedKeys) == 0 {
		return nil
	}

	existing, err := readFileOrEmpty(knownHostsPath)
	if err != nil {
		return err
	}

	// Remove old PKV managed block
	existing = removeKnownHostsBlock(existing)

	// Append new managed block
	var block strings.Builder
	block.WriteString("\n")
	block.WriteString(knownHostsMarkerStart)
	block.WriteString("\n")
	for _, k := range scannedKeys {
		block.WriteString(k)
		block.WriteString("\n")
	}
	block.WriteString(knownHostsMarkerEnd)
	block.WriteString("\n")

	content := strings.TrimRight(existing, "\n") + block.String()
	return os.WriteFile(knownHostsPath, []byte(content), 0600)
}

// RemoveKnownHosts removes the PKV managed block from ~/.ssh/known_hosts.
func RemoveKnownHosts(sshDir string) error {
	knownHostsPath := filepath.Join(sshDir, "known_hosts")

	existing, err := readFileOrEmpty(knownHostsPath)
	if err != nil {
		return err
	}
	if existing == "" {
		return nil
	}

	cleaned := removeKnownHostsBlock(existing)
	cleaned = collapseBlankLines(cleaned)
	return os.WriteFile(knownHostsPath, []byte(cleaned), 0600)
}

func removeKnownHostsBlock(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == knownHostsMarkerStart {
			inBlock = true
			continue
		}
		if trimmed == knownHostsMarkerEnd {
			inBlock = false
			continue
		}
		if !inBlock {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}
