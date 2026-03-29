package bw

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/shichao402/pkv/internal/bw/types"
)

type Client struct{}

func NewClient() *Client {
	return &Client{}
}

// EnsureUnlocked checks bw status, logs in if needed, unlocks vault, returns session key.
func (c *Client) EnsureUnlocked() (string, error) {
	if err := CheckBWInstalled(); err != nil {
		return "", err
	}

	status, err := c.getStatus()
	if err != nil {
		return "", fmt.Errorf("failed to get bw status: %w", err)
	}

	switch status.Status {
	case "unauthenticated":
		if err := c.login(); err != nil {
			return "", err
		}
		return c.unlock()
	case "locked":
		return c.unlock()
	case "unlocked":
		// Already unlocked, get session from env or unlock again
		if session := os.Getenv("BW_SESSION"); session != "" {
			return session, nil
		}
		return c.unlock()
	default:
		return "", fmt.Errorf("unknown bw status: %s", status.Status)
	}
}

// Sync syncs the vault with the remote server.
func (c *Client) Sync(session string) error {
	_, err := c.run(session, "sync")
	return err
}

// GetFolderID returns the folder ID for the given folder name.
func (c *Client) GetFolderID(session, name string) (string, error) {
	out, err := c.run(session, "list", "folders", "--search", name)
	if err != nil {
		return "", err
	}

	var folders []types.Folder
	if err := json.Unmarshal([]byte(out), &folders); err != nil {
		return "", fmt.Errorf("failed to parse folders: %w", err)
	}

	for _, f := range folders {
		if f.Name == name {
			return f.ID, nil
		}
	}
	return "", fmt.Errorf("folder '%s' not found", name)
}

// ListItems returns all items in the given folder.
func (c *Client) ListItems(session, folderID string) ([]types.Item, error) {
	out, err := c.run(session, "list", "items", "--folderid", folderID)
	if err != nil {
		return nil, err
	}

	var items []types.Item
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		return nil, fmt.Errorf("failed to parse items: %w", err)
	}
	return items, nil
}

// FilterSSHKeys returns only SSH key items (type=5) from the list.
func FilterSSHKeys(items []types.Item) []types.Item {
	var result []types.Item
	for _, item := range items {
		if item.Type == types.ItemTypeSSHKey {
			result = append(result, item)
		}
	}
	return result
}

// FilterSecureNotes returns only Secure Note items (type=2) from the list.
func FilterSecureNotes(items []types.Item) []types.Item {
	var result []types.Item
	for _, item := range items {
		if item.Type == types.ItemTypeSecureNote {
			result = append(result, item)
		}
	}
	return result
}

// FilterEnvNotes returns Secure Notes that are explicitly marked with pkv_type=env.
// Items without the pkv_type field or with a different value are excluded.
func FilterEnvNotes(items []types.Item) (matched []types.Item, skipped []types.Item) {
	for _, item := range items {
		if item.Type != types.ItemTypeSecureNote {
			continue
		}
		if item.IsEnv() {
			matched = append(matched, item)
		} else {
			skipped = append(skipped, item)
		}
	}
	return
}

// FilterNonEnvNotes returns Secure Notes that are NOT marked as pkv_type=env.
// This includes notes with no pkv_type field or any value other than "env".
func FilterNonEnvNotes(items []types.Item) []types.Item {
	var result []types.Item
	for _, item := range items {
		if item.Type == types.ItemTypeSecureNote && !item.IsEnv() {
			result = append(result, item)
		}
	}
	return result
}

func (c *Client) getStatus() (*types.Status, error) {
	cmd := exec.Command("bw", "status")
	cmd.Env = append(os.Environ(), "BW_NOINTERACTION=true")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bw status failed: %w", err)
	}

	var status types.Status
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, fmt.Errorf("failed to parse status: %w", err)
	}
	return &status, nil
}

func (c *Client) login() error {
	cmd := exec.Command("bw", "login")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *Client) unlock() (string, error) {
	cmd := exec.Command("bw", "unlock", "--raw")
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("bw unlock failed: %w", err)
	}
	session := strings.TrimSpace(string(out))
	if session == "" {
		return "", fmt.Errorf("bw unlock returned empty session")
	}
	return session, nil
}

func (c *Client) run(session string, args ...string) (string, error) {
	cmd := exec.Command("bw", args...)
	cmd.Env = append(os.Environ(), "BW_SESSION="+session)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("bw %s failed: %s", args[0], string(exitErr.Stderr))
		}
		return "", fmt.Errorf("bw %s failed: %w", args[0], err)
	}
	return string(out), nil
}
