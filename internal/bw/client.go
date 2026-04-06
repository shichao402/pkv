package bw

import (
	"encoding/base64"
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
		return c.unlockAndCache()
	case "locked":
		return c.unlockAndCache()
	case "unlocked":
		// Already unlocked, get session from env or unlock again
		if session := os.Getenv("BW_SESSION"); session != "" {
			return session, nil
		}
		return c.unlockAndCache()
	default:
		return "", fmt.Errorf("unknown bw status: %s", status.Status)
	}
}

// Sync syncs the vault with the remote server.
func (c *Client) Sync(session string) error {
	_, err := c.run(session, "sync")
	return err
}

// ListFolders returns all folders in the vault.
func (c *Client) ListFolders(session string) ([]types.Folder, error) {
	out, err := c.run(session, "list", "folders")
	if err != nil {
		return nil, err
	}

	var folders []types.Folder
	if err := json.Unmarshal([]byte(out), &folders); err != nil {
		return nil, fmt.Errorf("failed to parse folders: %w", err)
	}
	return folders, nil
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

// DeleteItem deletes a Bitwarden item by ID.
func (c *Client) DeleteItem(session, itemID string) error {
	_, err := c.run(session, "delete", "item", itemID)
	return err
}

// GetItem fetches a single Bitwarden item by ID.
func (c *Client) GetItem(session, itemID string) (types.Item, error) {
	out, err := c.run(session, "get", "item", itemID)
	if err != nil {
		return types.Item{}, err
	}

	var item types.Item
	if err := json.Unmarshal([]byte(out), &item); err != nil {
		return types.Item{}, fmt.Errorf("failed to parse item: %w", err)
	}
	return item, nil
}

// GetItemRaw fetches a single Bitwarden item by ID as raw JSON string.
func (c *Client) GetItemRaw(session, itemID string) (string, error) {
	out, err := c.run(session, "get", "item", itemID)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// CreateItem creates a Bitwarden item from JSON data, returns the created item's raw JSON output.
func (c *Client) CreateItem(session string, itemJSON []byte) (string, error) {
	encoded := base64Encode(itemJSON)
	out, err := c.run(session, "create", "item", encoded)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// EditItem updates an existing Bitwarden item with the given JSON data.
func (c *Client) EditItem(session, itemID string, itemJSON []byte) error {
	encoded := base64Encode(itemJSON)
	_, err := c.run(session, "edit", "item", itemID, encoded)
	return err
}

func base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
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
// The reserved name "pkv.env" is the primary convention; the legacy pkv_type=env
// marker is still accepted for compatibility during migration.
func FilterEnvNotes(items []types.Item) (matched, skipped []types.Item) {
	for _, item := range items {
		if item.Type != types.ItemTypeSecureNote {
			continue
		}
		if item.IsManagedEnvNote() {
			matched = append(matched, item)
		} else {
			skipped = append(skipped, item)
		}
	}
	return
}

// FindManagedEnvNote returns the single folder-level env note.
// Returns (zero, false, nil) when the folder has no env note.
func FindManagedEnvNote(items []types.Item) (types.Item, bool, error) {
	envNotes, _ := FilterEnvNotes(items)
	switch len(envNotes) {
	case 0:
		return types.Item{}, false, nil
	case 1:
		return envNotes[0], true, nil
	default:
		return types.Item{}, false, fmt.Errorf("found %d env notes in one folder; keep only one Secure Note named '%s'", len(envNotes), types.ReservedEnvNoteName)
	}
}

// FilterNonEnvNotes returns Secure Notes that are not treated as the folder-level env note.
func FilterNonEnvNotes(items []types.Item) []types.Item {
	var result []types.Item
	for _, item := range items {
		if item.Type == types.ItemTypeSecureNote && !item.IsManagedEnvNote() {
			result = append(result, item)
		}
	}
	return result
}

// FilterConfigNotes returns config-file notes stored as regular Secure Notes.
func FilterConfigNotes(items []types.Item) []types.Item {
	return FilterNonEnvNotes(items)
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

func (c *Client) unlockAndCache() (string, error) {
	session, err := c.unlock()
	if err != nil {
		return "", err
	}
	_ = os.Setenv("BW_SESSION", session)
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
