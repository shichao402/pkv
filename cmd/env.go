package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shichao402/pkv/internal/bw"
	"github.com/shichao402/pkv/internal/env"
	"github.com/shichao402/pkv/internal/pathutil"
	"github.com/shichao402/pkv/internal/securenote"
	"github.com/shichao402/pkv/internal/state"
)

var envCmd = &cobra.Command{
	Use:   "env <folder> [add|list|remove|edit|clean]",
	Short: "Manage environment variable notes in a Bitwarden folder",
	Long: `Manage environment variable Secure Notes in the specified Bitwarden folder.

Each Secure Note must have a custom field "pkv_type" set to "env" to be recognized.
The note content should contain KEY=VALUE pairs (one per line).
Supports: KEY=VALUE, export KEY=VALUE, # comments, quoted values.

On Windows, variables are set as persistent User environment variables.
On Linux/macOS, variables are written to ~/.pkv/env.sh and sourced from shell rc files.

Examples:
  pkv env github              Deploy env vars from the "github" folder
  pkv env github list         List env notes in the folder
  pkv env github add --name "tokens" --file ./.env   Add env note from file
  pkv env github add --name "secrets"                 Add env note via editor
  pkv env github edit <name-or-id>   Edit an env note in $EDITOR
  pkv env github remove <id>        Remove an env note from Bitwarden
  pkv env github clean              Remove deployed env vars from "github"`,
	Args:               cobra.MinimumNArgs(1),
	RunE:               runEnv,
	DisableFlagParsing: false,
}

var (
	envAddNameFlag string
	envAddFileFlag string
)

func init() {
	rootCmd.AddCommand(envCmd)
	envCmd.Flags().StringVar(&envAddNameFlag, "name", "", "Note name in Bitwarden (used with 'add')")
	envCmd.Flags().StringVar(&envAddFileFlag, "file", "", "File path to read content from (used with 'add')")
}

func runEnv(_ *cobra.Command, args []string) error {
	folder := args[0]

	if len(args) >= 2 {
		switch args[1] {
		case "clean":
			return runEnvClean(folder)
		case "list":
			return runEnvList(folder)
		case "add":
			return runEnvAdd(folder)
		case "remove":
			if len(args) < 3 {
				return fmt.Errorf("usage: pkv env <folder> remove <id> [id2] [id3]...")
			}
			return runEnvRemove(folder, args[2:])
		case "edit":
			if len(args) < 3 {
				return fmt.Errorf("usage: pkv env <folder> edit <name-or-id>")
			}
			return runEnvEdit(folder, args[2])
		default:
			return fmt.Errorf("unknown option: %s (expected 'add', 'list', 'remove', 'edit', or 'clean')", args[1])
		}
	}

	return runEnvDeploy(folder)
}

func runEnvDeploy(folder string) error {
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

	fmt.Println("Listing items...")
	items, err := client.ListItems(session, folderID)
	if err != nil {
		return fmt.Errorf("list items failed: %w", err)
	}

	notes, skipped := bw.FilterEnvNotes(items)
	if len(skipped) > 0 {
		fmt.Printf("Skipped %d note(s) without pkv_type=env:\n", len(skipped))
		for _, s := range skipped {
			fmt.Printf("  - '%s' (add custom field pkv_type=env in Bitwarden to include)\n", s.Name)
		}
	}
	if len(notes) == 0 {
		fmt.Println("No env notes found. Make sure Secure Notes have custom field pkv_type=env.")
		return nil
	}

	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state failed: %w", err)
	}

	deployer := env.NewDeployer(st, confirmEnvOverwrite)
	totalVars := 0
	for _, note := range notes {
		fmt.Printf("  Deploying '%s'...\n", note.Name)
		vars, err := deployer.Deploy(note)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Failed to deploy '%s': %v\n", note.Name, err)
			continue
		}
		for _, v := range vars {
			fmt.Printf("    + %s\n", v.Key)
		}
		totalVars += len(vars)
	}

	if err := st.Save(); err != nil {
		return fmt.Errorf("save state failed: %w", err)
	}

	fmt.Printf("Deployed %d variable(s). Open a new terminal to use them.\n", totalVars)
	return nil
}

func runEnvClean(folder string) error {
	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state failed: %w", err)
	}

	entries := st.FindEnvsByName(folder)
	if len(entries) == 0 {
		fmt.Printf("No environment variables found for folder '%s'.\n", folder)
		return nil
	}

	deployer := env.NewDeployer(st, nil)
	cleaned := 0
	for _, entry := range entries {
		fmt.Printf("  Removing '%s' (%s)...\n", entry.Name, strings.Join(entry.Keys, ", "))
		if err := deployer.Remove(entry); err != nil {
			fmt.Fprintf(os.Stderr, "  Failed to remove '%s': %v\n", entry.Name, err)
			continue
		}
		cleaned++
	}

	st.RemoveEnvsByName(folder)
	if err := st.Save(); err != nil {
		return fmt.Errorf("save state failed: %w", err)
	}

	fmt.Printf("Cleaned %d env group(s) for '%s'. Restart terminal to apply.\n", cleaned, folder)
	return nil
}

func runEnvList(folder string) error {
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

	envNotes, _ := bw.FilterEnvNotes(items)
	securenote.PrintList(envNotes, folder, "env notes")
	return nil
}

func runEnvAdd(folder string) error {
	name := envAddNameFlag
	if name == "" {
		return fmt.Errorf("--name is required: pkv env <folder> add --name <name> [--file <path>]")
	}

	var content string
	if envAddFileFlag != "" {
		// Read from file
		filePath := envAddFileFlag
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
		fmt.Println("Opening editor to write env content (KEY=VALUE format)...")
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

	fmt.Printf("Creating env note '%s'...\n", name)
	itemID, err := securenote.Add(client, session, folderID, name, content, true)
	if err != nil {
		return fmt.Errorf("create env note failed: %w", err)
	}

	fmt.Printf("Env note '%s' created with pkv_type=env (ID: %s)\n", name, itemID)
	return nil
}

func runEnvRemove(folder string, ids []string) error {
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

	envNotes, _ := bw.FilterEnvNotes(items)

	// Build lookup map
	noteMap := make(map[string]string) // id -> name
	for _, n := range envNotes {
		noteMap[n.ID] = n.Name
	}

	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state failed: %w", err)
	}

	fmt.Printf("Removing env notes from folder '%s'...\n", folder)
	removed := 0
	for _, id := range ids {
		name, found := noteMap[id]
		if !found {
			fmt.Fprintf(os.Stderr, "  Env note '%s' not found in folder '%s'\n", id, folder)
			continue
		}

		if err := client.DeleteItem(session, id); err != nil {
			fmt.Fprintf(os.Stderr, "  Failed to remove '%s' (%s): %v\n", name, id, err)
			continue
		}

		st.RemoveEnvByItemID(id)
		fmt.Printf("  Removed '%s' (%s)\n", name, id)
		removed++
	}

	if err := st.Save(); err != nil {
		return fmt.Errorf("save state failed: %w", err)
	}

	fmt.Printf("Removed %d env note(s).\n", removed)
	return nil
}

func runEnvEdit(folder string, nameOrID string) error {
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

	envNotes, _ := bw.FilterEnvNotes(items)
	item, err := securenote.ResolveItem(envNotes, nameOrID)
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
		fmt.Printf("Env note '%s' updated.\n", item.Name)
	}
	return nil
}

// confirmEnvOverwrite asks the user whether to overwrite conflicting keys.
func confirmEnvOverwrite(conflicts []env.ConflictInfo) (bool, error) {
	fmt.Println("  Conflicting environment variables detected:")
	for _, c := range conflicts {
		fmt.Printf("    %s: currently set by '%s'\n", c.Key, c.ExistingName)
	}
	fmt.Print("  Overwrite these variables? (y/N): ")

	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes", nil
}
