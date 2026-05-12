package ssh

import (
	"os"
	"path/filepath"
	"strings"
)

const knownHostsMarkerStart = "# >>> PKV MANAGED START <<<"
const knownHostsMarkerEnd = "# >>> PKV MANAGED END <<<"

// RemoveKnownHosts removes the PKV managed block from ~/.ssh/known_hosts.
//
// Historically PKV ran `ssh-keyscan` to prefill ~/.ssh/known_hosts during
// `pkv get`. That was removed in v0.9.x: the prefill had a high failure
// surface (non-22 ports, VPC-internal IPs, offline clients) and added no
// real security over OpenSSH's first-connect fingerprint prompt — both
// learn the host key from the host itself. We keep the cleanup half so
// users upgrading from older versions don't carry stale managed blocks.
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
	if cleaned == existing {
		// No managed block; don't touch the file (preserves mtime/perms).
		return nil
	}
	cleaned = collapseBlankLines(cleaned)
	return os.WriteFile(knownHostsPath, []byte(cleaned), 0o600)
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
