package ssh

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// AppendAuthorizedKey appends a public key line to ~/.ssh/authorized_keys on
// the local host, creating the file with mode 0o600 if needed.
//
// If a line with the same key type and key material is already present
// (comment is ignored when comparing), nothing is written and added=false is
// returned. The path returned is always the resolved authorized_keys path,
// even on error, so the caller can surface it in messages.
func AppendAuthorizedKey(publicKey string) (added bool, path string, err error) {
	publicKey = strings.TrimSpace(publicKey)
	if publicKey == "" {
		return false, "", fmt.Errorf("empty public key")
	}

	newType, newMaterial, ok := splitPubKey(publicKey)
	if !ok {
		return false, "", fmt.Errorf("malformed public key (need '<type> <base64>[ comment]')")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return false, "", fmt.Errorf("resolve home directory: %w", err)
	}
	sshDir := filepath.Join(home, ".ssh")
	path = filepath.Join(sshDir, "authorized_keys")

	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return false, path, fmt.Errorf("create %s: %w", sshDir, err)
	}

	// Check whether the key is already present.
	existing, readErr := os.ReadFile(path)
	if readErr != nil && !os.IsNotExist(readErr) {
		return false, path, fmt.Errorf("read %s: %w", path, readErr)
	}
	if hasAuthorizedKey(existing, newType, newMaterial) {
		return false, path, nil
	}

	// Ensure the file ends with a newline before appending.
	prefix := []byte{}
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		prefix = []byte{'\n'}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return false, path, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() {
		if closeErr := f.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close %s: %w", path, closeErr)
		}
	}()

	// Best-effort: tighten perms on a pre-existing file with looser mode.
	if info, statErr := f.Stat(); statErr == nil && info.Mode().Perm() != 0o600 {
		_ = f.Chmod(0o600)
	}

	if _, err := f.Write(prefix); err != nil {
		return false, path, fmt.Errorf("write %s: %w", path, err)
	}
	if _, err := f.WriteString(publicKey + "\n"); err != nil {
		return false, path, fmt.Errorf("write %s: %w", path, err)
	}
	return true, path, nil
}

// splitPubKey returns (type, base64Material, ok).
func splitPubKey(line string) (keyType, material string, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", "", false
	}
	return fields[0], fields[1], true
}

// hasAuthorizedKey reports whether content already contains a line whose key
// type and material match the given values (comments are ignored).
func hasAuthorizedKey(content []byte, keyType, material string) bool {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	// authorized_keys lines can be long (RSA 4096 in base64 is ~720 chars; option
	// strings push it further). Bump the buffer cap so very long lines parse.
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		t, m, ok := splitPubKey(line)
		if !ok {
			continue
		}
		if t == keyType && m == material {
			return true
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return false
	}
	return false
}
