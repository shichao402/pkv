package securenote

import (
	"encoding/json"
	"fmt"
	"unicode"
	"unicode/utf8"

	"github.com/shichao402/pkv/internal/bw"
	"github.com/shichao402/pkv/internal/bw/types"
)

// secureNoteItem is a minimal Bitwarden Secure Note for creation.
type secureNoteItem struct {
	Type       int           `json:"type"`
	Name       string        `json:"name"`
	FolderID   string        `json:"folderId,omitempty"`
	Notes      string        `json:"notes"`
	SecureNote secureNoteObj `json:"secureNote"`
	Fields     []customField `json:"fields,omitempty"`
}

type secureNoteObj struct {
	Type int `json:"type"` // 0 = Generic
}

type customField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Type  int    `json:"type"` // 0 = Text
}

// Add creates a new Secure Note in Bitwarden.
// If isEnv is true, a pkv_type=env custom field is added.
// Returns the created item's raw output (contains ID).
func Add(client *bw.Client, session, folderID, name, content string, isEnv bool) (string, error) {
	item := secureNoteItem{
		Type:       types.ItemTypeSecureNote,
		Name:       name,
		FolderID:   folderID,
		Notes:      content,
		SecureNote: secureNoteObj{Type: 0},
	}

	if isEnv {
		item.Fields = []customField{
			{Name: types.PKVFieldName, Value: types.PKVTypeEnv, Type: 0},
		}
	}

	itemJSON, err := json.Marshal(item)
	if err != nil {
		return "", fmt.Errorf("marshal item: %w", err)
	}

	out, err := client.CreateItem(session, itemJSON)
	if err != nil {
		return "", fmt.Errorf("create item: %w", err)
	}

	// Extract ID from the returned JSON
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		return "", fmt.Errorf("parse created item: %w", err)
	}

	return created.ID, nil
}

// Edit opens $EDITOR with item.Notes, and writes back to Bitwarden if changed.
// Returns true if the item was updated.
func Edit(client *bw.Client, session string, item types.Item) (bool, error) {
	edited, err := OpenEditor(item.Notes)
	if err != nil {
		return false, err
	}

	if edited == item.Notes {
		return false, nil
	}

	// Get the full item JSON for editing (bw edit requires full item)
	fullJSON, err := client.GetItemRaw(session, item.ID)
	if err != nil {
		return false, fmt.Errorf("get item for edit: %w", err)
	}

	// Parse, update Notes, re-marshal
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(fullJSON), &raw); err != nil {
		return false, fmt.Errorf("parse item JSON: %w", err)
	}
	raw["notes"] = edited

	updatedJSON, err := json.Marshal(raw)
	if err != nil {
		return false, fmt.Errorf("marshal updated item: %w", err)
	}

	if err := client.EditItem(session, item.ID, updatedJSON); err != nil {
		return false, fmt.Errorf("edit item: %w", err)
	}

	return true, nil
}

// ResolveItem finds an item by name or ID. Tries name first, then ID.
func ResolveItem(items []types.Item, nameOrID string) (types.Item, error) {
	// Try by name first
	for _, item := range items {
		if item.Name == nameOrID {
			return item, nil
		}
	}

	// Try by ID
	for _, item := range items {
		if item.ID == nameOrID {
			return item, nil
		}
	}

	return types.Item{}, fmt.Errorf("item '%s' not found (tried matching by name and ID)", nameOrID)
}

// FormatSize formats a byte count as a human-readable size string.
func FormatSize(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	kb := float64(n) / 1024
	if kb < 1024 {
		return fmt.Sprintf("%.1fKB", kb)
	}
	mb := kb / 1024
	return fmt.Sprintf("%.1fMB", mb)
}

// PrintList prints a table of Secure Notes.
func PrintList(items []types.Item, folder, label string) {
	if len(items) == 0 {
		fmt.Printf("No %s found in folder '%s'.\n", label, folder)
		return
	}

	// Calculate column widths
	nameWidth := 4 // "Name"
	for _, item := range items {
		if len(item.Name) > nameWidth {
			nameWidth = len(item.Name)
		}
	}

	fmt.Printf("\n%s in folder '%s':\n\n", capitalizeFirst(label), folder)
	fmt.Printf("%-36s  %-*s  %s\n", "ID", nameWidth, "Name", "Size")
	fmt.Printf("%-36s  %-*s  %s\n", "----", nameWidth, "----", "----")

	for _, item := range items {
		size := FormatSize(len(item.Notes))
		fmt.Printf("%-36s  %-*s  %s\n", item.ID, nameWidth, item.Name, size)
	}

	fmt.Printf("\n%d %s found.\n", len(items), label)
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}
