package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shichao402/pkv/internal/bw"
	"github.com/shichao402/pkv/internal/note"
	"github.com/shichao402/pkv/internal/pathutil"
	"github.com/shichao402/pkv/internal/securenote"
	"github.com/shichao402/pkv/internal/state"
)

var noteCmd = &cobra.Command{
	Use:   "note <folder> [add|list|remove|edit|clean]",
	Short: "Manage Secure Notes in a Bitwarden folder",
	Long: `Manage Secure Notes in the specified Bitwarden folder.

Notes with custom field "pkv_type" set to "env" are excluded (use 'pkv env' for those).

Examples:
  pkv note LyraX              Sync notes from "LyraX" to current directory
  pkv note LyraX list         List notes in the folder
  pkv note LyraX add --name "nginx.conf" --file ./nginx.conf   Add a note from file
  pkv note LyraX add --name "config.yaml"                      Add a note via editor
  pkv note LyraX edit <name-or-id>   Edit a note in $EDITOR
  pkv note LyraX remove <id>        Remove a note from Bitwarden
  pkv note LyraX clean              Remove previously synced note files`,
	Args:               cobra.MinimumNArgs(1),
	RunE:               runNote,
	DisableFlagParsing: false,
}

var (
	noteAddNameFlag string
	noteAddFileFlag string
)

func init() {
	rootCmd.AddCommand(noteCmd)
	noteCmd.Flags().StringVar(&noteAddNameFlag, "name", "", "Note name in Bitwarden (used with 'add')")
	noteCmd.Flags().StringVar(&noteAddFileFlag, "file", "", "File path to read content from (used with 'add')")
}

func runNote(_ *cobra.Command, args []string) error {
	folder := args[0]

	if len(args) >= 2 {
		switch args[1] {
		case "clean":
			return runNoteClean(folder)
		case "list":
			return runNoteList(folder)
		case "add":
			return runNoteAdd(folder)
		case "remove":
			if len(args) < 3 {
				return fmt.Errorf("usage: pkv note <folder> remove <id> [id2] [id3]...")
			}
			return runNoteRemove(folder, args[2:])
		case "edit":
			if len(args) < 3 {
				return fmt.Errorf("usage: pkv note <folder> edit <name-or-id>")
			}
			return runNoteEdit(folder, args[2])
		default:
			return fmt.Errorf("unknown option: %s (expected 'add', 'list', 'remove', 'edit', or 'clean')", args[1])
		}
	}

	return runNoteSync(folder)
}

func runNoteSync(folder string) error {
	client := bw.NewClient()

	fmt.Println("Authenticating with Bitwarden...")
	session, err := client.EnsureUnlocked()
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	fmt.Println("Syncing vault...")
	if err := client.Sync(session); err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	fmt.Printf("Looking up folder '%s'...\n", folder)
	folderID, err := client.GetFolderID(session, folder)
	if err != nil {
		return fmt.Errorf("folder lookup failed: %w", err)
	}

	fmt.Println("Listing notes...")
	items, err := client.ListItems(session, folderID)
	if err != nil {
		return fmt.Errorf("list items failed: %w", err)
	}

	notes := bw.FilterNonEnvNotes(items)
	if len(notes) == 0 {
		fmt.Println("No notes found in folder.")
		return nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory failed: %w", err)
	}

	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state failed: %w", err)
	}

	syncer := note.NewSyncer(st)
	synced := 0
	for _, n := range notes {
		fmt.Printf("  Syncing '%s'...\n", n.Name)
		if err := syncer.Sync(n, cwd, folder); err != nil {
			fmt.Fprintf(os.Stderr, "  Failed to sync '%s': %v\n", n.Name, err)
			continue
		}
		synced++
	}

	if err := st.Save(); err != nil {
		return fmt.Errorf("save state failed: %w", err)
	}

	fmt.Printf("Synced %d note(s) to %s\n", synced, cwd)
	return nil
}

func runNoteClean(folder string) error {
	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state failed: %w", err)
	}

	entries := st.FindSyncedNotesByFolder(folder)
	if len(entries) == 0 {
		for _, entry := range st.Notes {
			if entry.IsSynced() && entry.Folder == "" {
				fmt.Printf("No notes found for folder '%s'. Existing synced notes were created without folder metadata; sync them once again to enable folder-scoped clean.\n", folder)
				return nil
			}
		}
		fmt.Printf("No notes found for folder '%s'.\n", folder)
		return nil
	}

	syncer := note.NewSyncer(st)
	cleaned := 0
	for _, entry := range entries {
		fmt.Printf("  Removing '%s'...\n", entry.FileName)
		if err := syncer.Remove(entry); err != nil {
			fmt.Fprintf(os.Stderr, "  Failed to remove '%s': %v\n", entry.FileName, err)
			continue
		}
		st.RemoveNote(entry.ItemID)
		cleaned++
	}

	if err := st.Save(); err != nil {
		return fmt.Errorf("save state failed: %w", err)
	}

	fmt.Printf("Cleaned %d note(s) for folder '%s'.\n", cleaned, folder)
	return nil
}

func runNoteList(folder string) error {
	client := bw.NewClient()

	fmt.Println("Authenticating with Bitwarden...")
	session, err := client.EnsureUnlocked()
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	fmt.Println("Syncing vault...")
	if err := client.Sync(session); err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	fmt.Printf("Looking up folder '%s'...\n", folder)
	folderID, err := client.GetFolderID(session, folder)
	if err != nil {
		return fmt.Errorf("folder lookup failed: %w", err)
	}

	items, err := client.ListItems(session, folderID)
	if err != nil {
		return fmt.Errorf("list items failed: %w", err)
	}

	notes := bw.FilterNonEnvNotes(items)
	securenote.PrintList(notes, folder, "notes")
	return nil
}

func runNoteAdd(folder string) error {
	name := noteAddNameFlag
	if name == "" {
		return fmt.Errorf("--name is required: pkv note <folder> add --name <name> [--file <path>]")
	}

	var content string
	if noteAddFileFlag != "" {
		// Read from file
		filePath := noteAddFileFlag
		filePath, err := pathutil.ExpandTilde(filePath)
		if err != nil {
			return fmt.Errorf("resolve home directory: %w", err)
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
		content = string(data)
	} else {
		// Open editor
		fmt.Println("Opening editor to write note content...")
		edited, err := securenote.OpenEditor("")
		if err != nil {
			return fmt.Errorf("editor: %w", err)
		}
		if strings.TrimSpace(edited) == "" {
			fmt.Println("Empty content, cancelled.")
			return nil
		}
		content = edited
	}

	client := bw.NewClient()

	fmt.Println("Authenticating with Bitwarden...")
	session, err := client.EnsureUnlocked()
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	fmt.Println("Syncing vault...")
	if err := client.Sync(session); err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	fmt.Printf("Looking up folder '%s'...\n", folder)
	folderID, err := client.GetFolderID(session, folder)
	if err != nil {
		return fmt.Errorf("folder lookup failed: %w", err)
	}

	fmt.Printf("Creating note '%s'...\n", name)
	itemID, err := securenote.Add(client, session, folderID, name, content, false)
	if err != nil {
		return fmt.Errorf("create note failed: %w", err)
	}

	fmt.Printf("Note '%s' created (ID: %s)\n", name, itemID)
	return nil
}

func runNoteRemove(folder string, ids []string) error {
	client := bw.NewClient()

	fmt.Println("Authenticating with Bitwarden...")
	session, err := client.EnsureUnlocked()
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	fmt.Println("Syncing vault...")
	if err := client.Sync(session); err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	fmt.Printf("Looking up folder '%s'...\n", folder)
	folderID, err := client.GetFolderID(session, folder)
	if err != nil {
		return fmt.Errorf("folder lookup failed: %w", err)
	}

	items, err := client.ListItems(session, folderID)
	if err != nil {
		return fmt.Errorf("list items failed: %w", err)
	}

	notes := bw.FilterNonEnvNotes(items)

	// Build lookup map
	noteMap := make(map[string]string) // id -> name
	for _, n := range notes {
		noteMap[n.ID] = n.Name
	}

	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state failed: %w", err)
	}

	fmt.Printf("Removing notes from folder '%s'...\n", folder)
	removed := 0
	for _, id := range ids {
		name, found := noteMap[id]
		if !found {
			fmt.Fprintf(os.Stderr, "  Note '%s' not found in folder '%s'\n", id, folder)
			continue
		}

		if err := client.DeleteItem(session, id); err != nil {
			fmt.Fprintf(os.Stderr, "  Failed to remove '%s' (%s): %v\n", name, id, err)
			continue
		}

		st.RemoveNote(id)
		fmt.Printf("  Removed '%s' (%s)\n", name, id)
		removed++
	}

	if err := st.Save(); err != nil {
		return fmt.Errorf("save state failed: %w", err)
	}

	fmt.Printf("Removed %d note(s).\n", removed)
	return nil
}

func runNoteEdit(folder string, nameOrID string) error {
	client := bw.NewClient()

	fmt.Println("Authenticating with Bitwarden...")
	session, err := client.EnsureUnlocked()
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	fmt.Println("Syncing vault...")
	if err := client.Sync(session); err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	fmt.Printf("Looking up folder '%s'...\n", folder)
	folderID, err := client.GetFolderID(session, folder)
	if err != nil {
		return fmt.Errorf("folder lookup failed: %w", err)
	}

	items, err := client.ListItems(session, folderID)
	if err != nil {
		return fmt.Errorf("list items failed: %w", err)
	}

	notes := bw.FilterNonEnvNotes(items)
	item, err := securenote.ResolveItem(notes, nameOrID)
	if err != nil {
		return err
	}

	fmt.Printf("Editing '%s'...\n", item.Name)
	updated, err := securenote.Edit(client, session, item)
	if err != nil {
		return fmt.Errorf("edit failed: %w", err)
	}

	if !updated {
		fmt.Println("No changes made.")
	} else {
		fmt.Printf("Note '%s' updated.\n", item.Name)
	}
	return nil
}
