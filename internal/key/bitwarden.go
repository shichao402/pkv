package key

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shichao402/pkv/internal/bw"
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
func CreateBWSSHKey(client *bw.Client, session, name, folderID, privateKey, publicKey, fingerprint string) (string, error) {
	if client == nil {
		client = bw.NewClient()
	}

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

	output, err := client.CreateItem(session, []byte(jsonData))
	if err != nil {
		return "", fmt.Errorf("bw create item failed: %w", err)
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(output), &created); err != nil {
		return "", fmt.Errorf("parse created item: %w", err)
	}
	if created.ID == "" {
		return "", fmt.Errorf("created item response missing id")
	}

	return created.ID, nil
}
