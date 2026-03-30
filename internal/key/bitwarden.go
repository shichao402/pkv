package key

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
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
	Type   int       `json:"type"`
	Name   string    `json:"name"`
	Notes  string    `json:"notes,omitempty"`
	SSHKey *BWSSHKey `json:"sshKey"`
}

// CreateBWSSHKey creates an SSH Key item in Bitwarden vault.
// session is optional; if empty, it will be read from BW_SESSION environment variable.
func CreateBWSSHKey(session, name, privateKey, publicKey, fingerprint string) (string, error) {
	item := BWItem{
		Type: 5, // SSH Key item type
		Name: name,
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

// EnsureBWUnlocked ensures Bitwarden vault is unlocked and returns session key.
// If bw is not authenticated, it will prompt user to login.
// If bw is locked, it will prompt user for master password.
func EnsureBWUnlocked() (string, error) {
	// First check BW_SESSION environment variable
	if session := os.Getenv("BW_SESSION"); session != "" {
		// Try to use existing session
		cmd := exec.Command("bw", "status", "--session", session)
		output, err := cmd.CombinedOutput()
		if err == nil {
			// Verify the response contains proper JSON
			var result map[string]interface{}
			if json.Unmarshal(output, &result) == nil {
				if status, ok := result["status"].(string); ok && status == "unlocked" {
					return session, nil
				}
			}
		}
	}

	// Check current status
	cmd := exec.Command("bw", "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("bw status check failed: %w", err)
	}

	var result map[string]interface{}
	err = json.Unmarshal(output, &result)
	if err != nil {
		return "", fmt.Errorf("parse bw status failed: %w", err)
	}

	status, ok := result["status"].(string)
	if !ok {
		return "", fmt.Errorf("invalid bw status response")
	}

	// If unauthenticated, prompt to login
	if status == "unauthenticated" {
		return "", fmt.Errorf("not authenticated with Bitwarden. Please run 'bw login' first")
	}

	// If locked, prompt for master password
	if status == "locked" {
		fmt.Print("Bitwarden vault is locked. Enter your master password: ")
		var password string
		_, err := fmt.Scanln(&password)
		if err != nil {
			return "", fmt.Errorf("read password failed: %w", err)
		}

		cmd := exec.Command("bw", "unlock", password, "--raw")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("unlock failed: %w\n%s", err, string(output))
		}
		return strings.TrimSpace(string(output)), nil
	}

	// If already unlocked, return empty string (bw will use environment)
	return "", nil
}
