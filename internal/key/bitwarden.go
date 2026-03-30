package key

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// BWSSHKey represents a Bitwarden SSH Key item (type 5)
type BWSSHKey struct {
	PrivateKey     string `json:"privateKey"`
	PublicKey      string `json:"publicKey"`
	KeyFingerprint string `json:"keyFingerprint"`
}

// BWItem represents a Bitwarden item with SSH Key data
type BWItem struct {
	Type     int       `json:"type"`
	Name     string    `json:"name"`
	FolderID string    `json:"folderId,omitempty"`
	Notes    string    `json:"notes,omitempty"`
	SSHKey   *BWSSHKey `json:"sshKey"`
}

// CreateBWSSHKey creates an SSH Key item in Bitwarden vault.
// session is optional; if empty, it will be read from BW_SESSION environment variable.
// folderID is optional; if provided, the item will be placed in that folder.
func CreateBWSSHKey(session, name, folderID, privateKey, publicKey, fingerprint string) (string, error) {
	item := BWItem{
		Type:     5, // SSH Key item type
		Name:     name,
		FolderID: folderID,
		SSHKey: &BWSSHKey{
			PrivateKey:     strings.TrimSpace(privateKey),
			PublicKey:      strings.TrimSpace(publicKey),
			KeyFingerprint: fingerprint,
		},
	}

	jsonData, err := json.Marshal(item)
	if err != nil {
		return "", fmt.Errorf("marshal JSON failed: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(jsonData)

	args := []string{"create", "item", encoded}
	if session != "" {
		args = append(args, "--session", session)
	}

	cmd := exec.Command("bw", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("bw create item failed: %w\n%s", err, string(output))
	}

	return strings.TrimSpace(string(output)), nil
}
