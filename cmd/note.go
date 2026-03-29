package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shichao402/pkv/internal/bw"
	"github.com/shichao402/pkv/internal/note"
	"github.com/shichao402/pkv/internal/state"
)

var noteCmd = &cobra.Command{
	Use:   "note <folder> [clean]",
	Short: "Sync config notes from a Bitwarden folder to current directory",
	Long: `Sync all Secure Notes from the specified Bitwarden folder as files in the current directory.

Notes with custom field "pkv_type" set to "env" are excluded (use 'pkv env' for those).

Examples:
  pkv note LyraX          Sync notes from "LyraX" to current directory
  pkv note LyraX clean    Remove previously synced note files`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runNote,
}

func init() {
	rootCmd.AddCommand(noteCmd)
}

func runNote(_ *cobra.Command, args []string) error {
	folder := args[0]

	if len(args) == 2 {
		return handleCleanCommand(args[1], func() error { return runNoteClean(folder) })
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
		if err := syncer.Sync(n, cwd); err != nil {
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

	if len(st.Notes) == 0 {
		fmt.Println("No notes to clean.")
		return nil
	}

	syncer := note.NewSyncer(st)
	cleaned := 0
	for _, entry := range st.Notes {
		fmt.Printf("  Removing '%s'...\n", entry.FileName)
		if err := syncer.Remove(entry); err != nil {
			fmt.Fprintf(os.Stderr, "  Failed to remove '%s': %v\n", entry.FileName, err)
			continue
		}
		cleaned++
	}

	st.Notes = nil
	if err := st.Save(); err != nil {
		return fmt.Errorf("save state failed: %w", err)
	}

	fmt.Printf("Cleaned %d note(s).\n", cleaned)
	return nil
}
