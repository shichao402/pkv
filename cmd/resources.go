package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shichao402/pkv/internal/bw"
	bwtypes "github.com/shichao402/pkv/internal/bw/types"
	"github.com/shichao402/pkv/internal/env"
	"github.com/shichao402/pkv/internal/key"
	"github.com/shichao402/pkv/internal/note"
	"github.com/shichao402/pkv/internal/pathutil"
	"github.com/shichao402/pkv/internal/securenote"
	"github.com/shichao402/pkv/internal/ssh"
	"github.com/shichao402/pkv/internal/state"
)

var (
	addSSHPrivFlag string
	addSSHPubFlag  string
	addNameFlag    string

	addNoteFileFlag string
)

var listCmd = &cobra.Command{
	Use:     "list [folder]",
	Short:   "List folders or resources in a folder",
	Example: "  pkv list\n  pkv list prod",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		switch len(args) {
		case 0:
			return listFoldersCmd.RunE(listFoldersCmd, nil)
		case 1:
			return listFolderCmd.RunE(listFolderCmd, args)
		default:
			return fmt.Errorf("usage: pkv list [folder]")
		}
	},
}

var listFoldersCmd = &cobra.Command{
	Use:   "folders",
	Short: "List Bitwarden folders",
	RunE: func(_ *cobra.Command, _ []string) error {
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

		folders, err := client.ListFolders(session)
		if err != nil {
			return fmt.Errorf("list folders failed: %w", err)
		}
		if len(folders) == 0 {
			fmt.Println("No folders found.")
			return nil
		}

		fmt.Println()
		fmt.Println("Folders:")
		fmt.Println()
		fmt.Printf("%-36s  %s\n", "ID", "Name")
		fmt.Printf("%-36s  %s\n", "----", "----")
		for _, folder := range folders {
			fmt.Printf("%-36s  %s\n", folder.ID, folder.Name)
		}
		fmt.Printf("\n%d folder(s) found.\n", len(folders))
		return nil
	},
}

var listFolderCmd = &cobra.Command{
	Use:   "folder <folder>",
	Short: "List resources inside one folder",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		folder := args[0]
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

		sshKeys := bw.FilterSSHKeys(items)
		envItem, hasEnv, err := bw.FindManagedEnvNote(items)
		if err != nil {
			return err
		}
		notes := bw.FilterConfigNotes(items)

		fmt.Printf("\nFolder '%s'\n\n", folder)
		fmt.Printf("SSH keys: %d\n", len(sshKeys))
		if hasEnv {
			fmt.Printf("Env note: %s (%s)\n", envItem.Name, envItem.ID)
		} else {
			fmt.Printf("Env note: none (create one named '%s')\n", bwtypes.ReservedEnvNoteName)
		}
		fmt.Printf("Config notes: %d\n", len(notes))

		if len(sshKeys) > 0 {
			fmt.Println("\nSSH:")
			for _, item := range sshKeys {
				fmt.Printf("  %s  %s\n", item.ID, item.Name)
			}
		}
		if len(notes) > 0 {
			fmt.Println("\nNotes:")
			for _, item := range notes {
				fmt.Printf("  %s  %s\n", item.ID, item.Name)
			}
		}
		return nil
	},
}

var getCmd = &cobra.Command{
	Use:     "get <folder> <ssh|env|note>",
	Short:   "Get resources from a Bitwarden folder",
	Example: "  pkv get prod ssh\n  pkv get prod env\n  pkv get prod note",
	Args:    cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		folder, kind := args[0], args[1]
		switch kind {
		case "ssh":
			return getSSHCmd.RunE(getSSHCmd, []string{folder})
		case "env":
			return getEnvCmd.RunE(getEnvCmd, []string{folder})
		case "note":
			return getNoteCmd.RunE(getNoteCmd, []string{folder})
		default:
			return fmt.Errorf("unknown resource type: %s (expected ssh, env, or note)", kind)
		}
	},
}

var getSSHCmd = &cobra.Command{
	Use:   "ssh <folder>",
	Short: "Deploy SSH keys from a folder",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		folder := args[0]
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

		fmt.Println("Listing SSH keys...")
		items, err := client.ListItems(session, folderID)
		if err != nil {
			return fmt.Errorf("list items failed: %w", err)
		}

		sshKeys := bw.FilterSSHKeys(items)

		st, err := state.Load()
		if err != nil {
			return fmt.Errorf("load state failed: %w", err)
		}

		deployer, err := ssh.NewDeployer(st)
		if err != nil {
			return fmt.Errorf("create ssh deployer failed: %w", err)
		}
		existing := st.FindDeployedSSHKeysByFolder(folder)
		existingByID := make(map[string]state.SSHKeyEntry, len(existing))
		for _, entry := range existing {
			existingByID[entry.ItemID] = entry
		}

		remoteByID := make(map[string]bwtypes.Item, len(sshKeys))
		for _, keyItem := range sshKeys {
			remoteByID[keyItem.ID] = keyItem
		}

		for _, entry := range existing {
			if _, ok := remoteByID[entry.ItemID]; ok {
				continue
			}
			fmt.Printf("  Removing stale '%s'...\n", entry.KeyName)
			if err := deployer.Remove(entry); err != nil {
				return fmt.Errorf("remove stale key '%s': %w", entry.KeyName, err)
			}
			st.RemoveStoredSSHKey(entry.ItemID)
		}

		deployed := 0
		for _, keyItem := range sshKeys {
			if entry, ok := existingByID[keyItem.ID]; ok && entry.KeyName != sanitizeSSHKeyName(keyItem.Name) {
				if err := deployer.Remove(entry); err != nil {
					return fmt.Errorf("refresh renamed key '%s': %w", entry.KeyName, err)
				}
				st.RemoveStoredSSHKey(entry.ItemID)
			}

			fmt.Printf("  Deploying '%s'...\n", keyItem.Name)
			if err := deployer.Deploy(keyItem, folder); err != nil {
				fmt.Fprintf(os.Stderr, "  Failed to deploy '%s': %v\n", keyItem.Name, err)
				continue
			}
			deployed++
		}

		allHosts := collectDeployedSSHHosts(st.SSHKeys)
		if len(allHosts) == 0 {
			fmt.Println("Cleaning known_hosts...")
			if err := deployer.RemoveAllKnownHosts(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: known_hosts cleanup failed: %v\n", err)
			}
		} else {
			fmt.Println("Rebuilding known_hosts...")
			if err := deployer.DeployKnownHosts(allHosts); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: known_hosts update failed: %v\n", err)
			}
		}

		if err := st.Save(); err != nil {
			return fmt.Errorf("save state failed: %w", err)
		}

		if len(sshKeys) == 0 {
			fmt.Println("No SSH keys found in folder.")
			return nil
		}
		fmt.Printf("Deployed %d SSH key(s).\n", deployed)
		return nil
	},
}

var getEnvCmd = &cobra.Command{
	Use:   "env <folder>",
	Short: "Materialize env artifacts for a folder",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		folder := args[0]
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

		st, err := state.Load()
		if err != nil {
			return fmt.Errorf("load state failed: %w", err)
		}

		deployer := env.NewDeployer(st)
		envItem, found, err := bw.FindManagedEnvNote(items)
		if err != nil {
			return err
		}
		if !found {
			cleaned := 0
			for _, entry := range st.FindEnvsByFolder(folder) {
				if err := deployer.Remove(entry); err != nil {
					return err
				}
				cleaned++
			}
			st.RemoveEnvsByFolder(folder)
			if err := st.Save(); err != nil {
				return fmt.Errorf("save state failed: %w", err)
			}
			if cleaned > 0 {
				fmt.Printf("No env note found. Cleaned %d local env artifact set(s) for folder '%s'.\n", cleaned, folder)
			} else {
				fmt.Printf("No env note found. Create one Secure Note named '%s'.\n", bwtypes.ReservedEnvNoteName)
			}
			return nil
		}

		entry, err := deployer.Deploy(folder, envItem)
		if err != nil {
			return err
		}
		if err := st.Save(); err != nil {
			return fmt.Errorf("save state failed: %w", err)
		}

		fmt.Printf("Wrote env artifacts for folder '%s'.\n", folder)
		fmt.Printf("  JSON: %s\n", entry.JSONPath)
		fmt.Printf("  Shell: %s\n", entry.ShellPath)
		fmt.Printf("  PowerShell: %s\n", entry.PowerShellPath)
		return nil
	},
}

var getNoteCmd = &cobra.Command{
	Use:   "note <folder>",
	Short: "Sync config notes from a folder into the current directory",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		folder := args[0]
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

		notes := bw.FilterConfigNotes(items)
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory failed: %w", err)
		}
		st, err := state.Load()
		if err != nil {
			return fmt.Errorf("load state failed: %w", err)
		}

		syncer := note.NewSyncer(st)
		synced, err := syncer.SyncFolder(notes, cwd, folder)
		if err != nil {
			return err
		}
		if err := st.Save(); err != nil {
			return fmt.Errorf("save state failed: %w", err)
		}

		if len(notes) == 0 {
			fmt.Printf("No config notes found in folder '%s'.\n", folder)
			return nil
		}
		fmt.Printf("Synced %d note(s) to %s\n", synced, cwd)
		return nil
	},
}

var addCmd = &cobra.Command{
	Use:     "add <folder> <ssh|env|note>",
	Short:   "Create resources in a Bitwarden folder",
	Example: "  pkv add prod ssh --priv ~/.ssh/id_ed25519 --name github\n  pkv add prod env --file .env.prod\n  pkv add prod note --name app.secrets.json --file ./app.secrets.json",
	Args:    cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		folder, kind := args[0], args[1]
		switch kind {
		case "ssh":
			return addSSHCmd.RunE(addSSHCmd, []string{folder})
		case "env":
			return addEnvCmd.RunE(addEnvCmd, []string{folder})
		case "note":
			return addNoteCmd.RunE(addNoteCmd, []string{folder})
		default:
			return fmt.Errorf("unknown resource type: %s (expected ssh, env, or note)", kind)
		}
	},
}

var addSSHCmd = &cobra.Command{
	Use:   "ssh <folder>",
	Short: "Add an SSH key to a folder",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		folder := args[0]
		cfg := &key.InputConfig{
			PrivatePath: addSSHPrivFlag,
			PublicKey:   addSSHPubFlag,
			KeyName:     addNameFlag,
			Folder:      folder,
		}

		expandedPath, err := pathutil.ExpandTilde(cfg.PrivatePath)
		if err != nil {
			return fmt.Errorf("resolve home directory: %w", err)
		}
		cfg.PrivatePath = expandedPath

		fmt.Printf("Adding SSH key to Bitwarden folder '%s'...\n", folder)
		if err := key.InteractiveInput(cfg); err != nil {
			return fmt.Errorf("input failed: %w", err)
		}

		fmt.Printf("\nReading private key: %s\n", cfg.PrivatePath)
		privateKeyBytes, err := os.ReadFile(cfg.PrivatePath)
		if err != nil {
			return fmt.Errorf("read private key failed: %w", err)
		}

		fmt.Println("Parsing and converting key...")
		opensshKey, publicKey, fingerprint, err := key.ParseAndConvertKey(privateKeyBytes)
		if err != nil {
			return fmt.Errorf("parse key failed: %w", err)
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

		confirm, err := key.ConfirmAndCreate(cfg, fingerprint)
		if err != nil {
			return fmt.Errorf("confirmation failed: %w", err)
		}
		if !confirm {
			fmt.Println("Cancelled.")
			return nil
		}

		fmt.Println("Creating SSH key in Bitwarden...")
		output, err := key.CreateBWSSHKey(client, session, cfg.KeyName, folderID, opensshKey, publicKey, fingerprint)
		if err != nil {
			return fmt.Errorf("create SSH key failed: %w", err)
		}

		st, err := state.Load()
		if err != nil {
			return fmt.Errorf("load state failed: %w", err)
		}
		st.AddStoredSSHKey(output, cfg.KeyName, fingerprint)
		if err := st.Save(); err != nil {
			return fmt.Errorf("save state failed: %w", err)
		}

		fmt.Printf("\nSSH key '%s' added to folder '%s'\n", cfg.KeyName, folder)
		fmt.Printf("  Fingerprint: %s\n", fingerprint)
		return nil
	},
}

var addEnvCmd = &cobra.Command{
	Use:   "env <folder>",
	Short: "Create or replace the folder env note",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		folder := args[0]
		content, err := readNoteContent(addNoteFileFlag, "Opening editor to write env content (KEY=VALUE format)...")
		if err != nil {
			return err
		}
		if strings.TrimSpace(content) == "" {
			fmt.Println("Empty content, cancelled.")
			return nil
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

		items, err := client.ListItems(session, folderID)
		if err != nil {
			return fmt.Errorf("list items failed: %w", err)
		}

		existing, found, err := bw.FindManagedEnvNote(items)
		if err != nil {
			return err
		}
		if found {
			fmt.Printf("Updating env note '%s'...\n", existing.Name)
			if err := securenote.UpdateContent(client, session, existing.ID, content); err != nil {
				return err
			}
			fmt.Printf("Env note '%s' updated.\n", existing.Name)
			return nil
		}

		fmt.Printf("Creating env note '%s'...\n", bwtypes.ReservedEnvNoteName)
		itemID, err := securenote.Add(client, session, folderID, bwtypes.ReservedEnvNoteName, content)
		if err != nil {
			return fmt.Errorf("create env note failed: %w", err)
		}
		fmt.Printf("Env note '%s' created (ID: %s)\n", bwtypes.ReservedEnvNoteName, itemID)
		return nil
	},
}

var addNoteCmd = &cobra.Command{
	Use:   "note <folder>",
	Short: "Create a config note in a folder",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		folder := args[0]
		if addNameFlag == "" {
			return fmt.Errorf("--name is required: pkv add <folder> note --name <name> [--file <path>]")
		}
		if addNameFlag == bwtypes.ReservedEnvNoteName {
			return fmt.Errorf("note name '%s' is reserved for folder env data", bwtypes.ReservedEnvNoteName)
		}

		content, err := readNoteContent(addNoteFileFlag, "Opening editor to write note content...")
		if err != nil {
			return err
		}
		if strings.TrimSpace(content) == "" {
			fmt.Println("Empty content, cancelled.")
			return nil
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

		fmt.Printf("Creating note '%s'...\n", addNameFlag)
		itemID, err := securenote.Add(client, session, folderID, addNameFlag, content)
		if err != nil {
			return fmt.Errorf("create note failed: %w", err)
		}
		fmt.Printf("Note '%s' created (ID: %s)\n", addNameFlag, itemID)
		return nil
	},
}

var editCmd = &cobra.Command{
	Use:     "edit <folder> <env|note> [name-or-id]",
	Short:   "Edit resources in a Bitwarden folder",
	Example: "  pkv edit prod env\n  pkv edit prod note app.secrets.json",
	Args:    cobra.MinimumNArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		folder, kind := args[0], args[1]
		switch kind {
		case "env":
			if len(args) != 2 {
				return fmt.Errorf("usage: pkv edit <folder> env")
			}
			return editEnvCmd.RunE(editEnvCmd, []string{folder})
		case "note":
			if len(args) != 3 {
				return fmt.Errorf("usage: pkv edit <folder> note <name-or-id>")
			}
			return editNoteCmd.RunE(editNoteCmd, []string{folder, args[2]})
		default:
			return fmt.Errorf("unknown resource type: %s (expected env or note)", kind)
		}
	},
}

var editEnvCmd = &cobra.Command{
	Use:   "env <folder>",
	Short: "Edit the folder env note",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		folder := args[0]
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

		item, found, err := bw.FindManagedEnvNote(items)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("no env note found in folder '%s' (expected Secure Note named '%s')", folder, bwtypes.ReservedEnvNoteName)
		}

		fmt.Printf("Editing '%s'...\n", item.Name)
		updated, err := securenote.Edit(client, session, item)
		if err != nil {
			return fmt.Errorf("edit failed: %w", err)
		}
		if !updated {
			fmt.Println("No changes made.")
			return nil
		}
		fmt.Printf("Env note '%s' updated.\n", item.Name)
		return nil
	},
}

var editNoteCmd = &cobra.Command{
	Use:   "note <folder> <name-or-id>",
	Short: "Edit a config note in a folder",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		folder := args[0]
		nameOrID := args[1]
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

		item, err := securenote.ResolveItem(bw.FilterConfigNotes(items), nameOrID)
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
			return nil
		}
		fmt.Printf("Note '%s' updated.\n", item.Name)
		return nil
	},
}

var removeCmd = &cobra.Command{
	Use:     "remove <folder> <ssh|env|note> [id...]",
	Short:   "Remove resources from Bitwarden",
	Example: "  pkv remove prod env\n  pkv remove prod ssh <item-id>\n  pkv remove prod note <item-id>",
	Args:    cobra.MinimumNArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		folder, kind := args[0], args[1]
		switch kind {
		case "env":
			if len(args) != 2 {
				return fmt.Errorf("usage: pkv remove <folder> env")
			}
			return removeEnvCmd.RunE(removeEnvCmd, []string{folder})
		case "ssh":
			if len(args) < 3 {
				return fmt.Errorf("usage: pkv remove <folder> ssh <id> [id2]...")
			}
			return removeSSHCmd.RunE(removeSSHCmd, append([]string{folder}, args[2:]...))
		case "note":
			if len(args) < 3 {
				return fmt.Errorf("usage: pkv remove <folder> note <id> [id2]...")
			}
			return removeNoteCmd.RunE(removeNoteCmd, append([]string{folder}, args[2:]...))
		default:
			return fmt.Errorf("unknown resource type: %s (expected ssh, env, or note)", kind)
		}
	},
}

var removeSSHCmd = &cobra.Command{
	Use:   "ssh <folder> <id> [id2]...",
	Short: "Remove SSH keys from a folder",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		folder := args[0]
		keyIDs := args[1:]
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
		sshKeys := bw.FilterSSHKeys(items)
		keyMap := make(map[string]string)
		for _, item := range sshKeys {
			keyMap[item.ID] = item.Name
		}

		st, err := state.Load()
		if err != nil {
			return fmt.Errorf("load state failed: %w", err)
		}

		deployer, err := ssh.NewDeployer(st)
		if err != nil {
			return fmt.Errorf("create ssh deployer failed: %w", err)
		}
		deployedByID := make(map[string]state.SSHKeyEntry)
		for _, entry := range st.FindDeployedSSHKeysByFolder(folder) {
			deployedByID[entry.ItemID] = entry
		}

		fmt.Printf("Removing SSH keys from folder '%s'...\n", folder)
		removed := 0
		for _, id := range keyIDs {
			name, found := keyMap[id]
			if !found {
				fmt.Fprintf(os.Stderr, "  Key '%s' not found in folder '%s'\n", id, folder)
				continue
			}
			if err := client.DeleteItem(session, id); err != nil {
				fmt.Fprintf(os.Stderr, "  Failed to remove '%s' (%s): %v\n", name, id, err)
				continue
			}
			cleanupFailed := false
			if entry, ok := deployedByID[id]; ok {
				if err := deployer.Remove(entry); err != nil {
					fmt.Fprintf(os.Stderr, "  Failed to clean local '%s': %v\n", name, err)
					cleanupFailed = true
				}
			}
			if !cleanupFailed {
				st.RemoveStoredSSHKey(id)
			}
			fmt.Printf("  Removed '%s' (%s)\n", name, id)
			removed++
		}

		remainingHosts := collectDeployedSSHHosts(st.SSHKeys)
		if len(remainingHosts) == 0 {
			if err := deployer.RemoveAllKnownHosts(); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: known_hosts cleanup failed: %v\n", err)
			}
		} else if err := deployer.DeployKnownHosts(remainingHosts); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: known_hosts rebuild failed: %v\n", err)
		}

		if err := st.Save(); err != nil {
			return fmt.Errorf("save state failed: %w", err)
		}
		fmt.Printf("Removed %d SSH key(s).\n", removed)
		return nil
	},
}

var removeEnvCmd = &cobra.Command{
	Use:   "env <folder>",
	Short: "Remove the folder env note from Bitwarden",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		folder := args[0]
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
		item, found, err := bw.FindManagedEnvNote(items)
		if err != nil {
			return err
		}
		if !found {
			fmt.Printf("No env note found in folder '%s'.\n", folder)
			return nil
		}

		if err := client.DeleteItem(session, item.ID); err != nil {
			return fmt.Errorf("remove env note failed: %w", err)
		}

		st, err := state.Load()
		if err != nil {
			return fmt.Errorf("load state failed: %w", err)
		}
		deployer := env.NewDeployer(st)
		entries := st.FindEnvsByFolder(folder)
		cleanupFailed := false
		for _, entry := range entries {
			if err := deployer.Remove(entry); err != nil {
				fmt.Fprintf(os.Stderr, "  Failed to clean local env artifacts for '%s': %v\n", entry.Name, err)
				cleanupFailed = true
			}
		}
		if !cleanupFailed {
			st.RemoveEnvsByFolder(folder)
		}
		if err := st.Save(); err != nil {
			return fmt.Errorf("save state failed: %w", err)
		}

		fmt.Printf("Removed env note '%s' (%s).\n", item.Name, item.ID)
		return nil
	},
}

var removeNoteCmd = &cobra.Command{
	Use:   "note <folder> <id> [id2]...",
	Short: "Remove config notes from a folder",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		folder := args[0]
		ids := args[1:]
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

		notes := bw.FilterConfigNotes(items)
		noteMap := make(map[string]string)
		for _, item := range notes {
			noteMap[item.ID] = item.Name
		}

		st, err := state.Load()
		if err != nil {
			return fmt.Errorf("load state failed: %w", err)
		}
		syncer := note.NewSyncer(st)

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
			cleanupFailed := false
			for _, entry := range st.Notes {
				if entry.ItemID != id {
					continue
				}
				if err := syncer.Remove(entry); err != nil {
					fmt.Fprintf(os.Stderr, "  Failed to clean local '%s': %v\n", entry.FilePath, err)
					cleanupFailed = true
				}
			}
			if !cleanupFailed {
				st.RemoveNote(id)
			}
			fmt.Printf("  Removed '%s' (%s)\n", name, id)
			removed++
		}

		if err := st.Save(); err != nil {
			return fmt.Errorf("save state failed: %w", err)
		}
		fmt.Printf("Removed %d note(s).\n", removed)
		return nil
	},
}

var cleanCmd = &cobra.Command{
	Use:     "clean <folder> <ssh|env|note>",
	Short:   "Clean local materialized resources",
	Example: "  pkv clean prod ssh\n  pkv clean prod env\n  pkv clean prod note",
	Args:    cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		folder, kind := args[0], args[1]
		switch kind {
		case "ssh":
			return cleanSSHCmd.RunE(cleanSSHCmd, []string{folder})
		case "env":
			return cleanEnvCmd.RunE(cleanEnvCmd, []string{folder})
		case "note":
			return cleanNoteCmd.RunE(cleanNoteCmd, []string{folder})
		default:
			return fmt.Errorf("unknown resource type: %s (expected ssh, env, or note)", kind)
		}
	},
}

var cleanSSHCmd = &cobra.Command{
	Use:   "ssh <folder>",
	Short: "Clean locally deployed SSH keys for a folder",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		folder := args[0]
		st, err := state.Load()
		if err != nil {
			return fmt.Errorf("load state failed: %w", err)
		}

		entries := st.FindDeployedSSHKeysByFolder(folder)
		if len(entries) == 0 {
			fmt.Printf("No SSH keys found for folder '%s'.\n", folder)
			return nil
		}

		deployer, err := ssh.NewDeployer(st)
		if err != nil {
			return fmt.Errorf("create ssh deployer failed: %w", err)
		}
		cleaned := 0
		for _, entry := range entries {
			fmt.Printf("  Removing '%s'...\n", entry.KeyName)
			if err := deployer.Remove(entry); err != nil {
				fmt.Fprintf(os.Stderr, "  Failed to remove '%s': %v\n", entry.KeyName, err)
				continue
			}
			st.RemoveStoredSSHKey(entry.ItemID)
			cleaned++
		}

		remainingHosts := collectDeployedSSHHosts(st.SSHKeys)
		if len(remainingHosts) == 0 {
			fmt.Println("  Cleaning known_hosts...")
			if err := deployer.RemoveAllKnownHosts(); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: known_hosts cleanup failed: %v\n", err)
			}
		} else {
			fmt.Println("  Rebuilding known_hosts...")
			if err := deployer.DeployKnownHosts(remainingHosts); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: known_hosts rebuild failed: %v\n", err)
			}
		}

		if err := st.Save(); err != nil {
			return fmt.Errorf("save state failed: %w", err)
		}
		fmt.Printf("Cleaned %d SSH key(s) for folder '%s'.\n", cleaned, folder)
		return nil
	},
}

var cleanEnvCmd = &cobra.Command{
	Use:   "env <folder>",
	Short: "Clean local env artifacts for a folder",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		folder := args[0]
		st, err := state.Load()
		if err != nil {
			return fmt.Errorf("load state failed: %w", err)
		}

		entries := st.FindEnvsByFolder(folder)
		if len(entries) == 0 {
			fmt.Printf("No env artifacts found for folder '%s'.\n", folder)
			return nil
		}

		deployer := env.NewDeployer(st)
		cleaned := 0
		for _, entry := range entries {
			fmt.Printf("  Removing env artifacts for '%s'...\n", entry.Name)
			if err := deployer.Remove(entry); err != nil {
				fmt.Fprintf(os.Stderr, "  Failed to remove '%s': %v\n", entry.Name, err)
				continue
			}
			cleaned++
		}

		allCleaned := cleaned == len(entries)
		if allCleaned {
			st.RemoveEnvsByFolder(folder)
		} else {
			fmt.Fprintln(os.Stderr, "Some env artifacts could not be removed; state was kept so you can retry clean.")
		}
		if err := st.Save(); err != nil {
			return fmt.Errorf("save state failed: %w", err)
		}
		fmt.Printf("Cleaned %d env artifact set(s) for '%s'.\n", cleaned, folder)
		return nil
	},
}

var cleanNoteCmd = &cobra.Command{
	Use:   "note <folder>",
	Short: "Clean synced config notes for the current directory",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		folder := args[0]
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory failed: %w", err)
		}
		absDir, err := filepath.Abs(cwd)
		if err == nil {
			cwd = absDir
		}

		st, err := state.Load()
		if err != nil {
			return fmt.Errorf("load state failed: %w", err)
		}

		entries := st.FindSyncedNotes(folder, cwd)
		if len(entries) == 0 {
			fmt.Printf("No synced notes found for folder '%s' in %s.\n", folder, cwd)
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
			st.RemoveNoteForTarget(entry.ItemID, folder, cwd)
			cleaned++
		}

		if err := st.Save(); err != nil {
			return fmt.Errorf("save state failed: %w", err)
		}
		fmt.Printf("Cleaned %d note(s) for folder '%s' in %s.\n", cleaned, folder, cwd)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd, getCmd, addCmd, editCmd, removeCmd, cleanCmd)

	addCmd.Flags().StringVar(&addSSHPrivFlag, "priv", "", "Private key file path (used with ssh)")
	addCmd.Flags().StringVar(&addSSHPubFlag, "pub", "", "Public key, ssh-rsa AAAA... format (used with ssh)")
	addCmd.Flags().StringVar(&addNameFlag, "name", "", "Item name in Bitwarden (used with ssh/note)")
	addCmd.Flags().StringVar(&addNoteFileFlag, "file", "", "File path to read content from (used with env/note)")
}

func readNoteContent(fileFlag, openEditorMessage string) (string, error) {
	if fileFlag != "" {
		filePath, err := pathutil.ExpandTilde(fileFlag)
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("read file: %w", err)
		}
		return string(data), nil
	}

	fmt.Println(openEditorMessage)
	edited, err := securenote.OpenEditor("")
	if err != nil {
		return "", fmt.Errorf("editor: %w", err)
	}
	return edited, nil
}

func collectDeployedSSHHosts(entries []state.SSHKeyEntry) []string {
	var hosts []string
	for _, entry := range entries {
		if !entry.IsDeployed() || len(entry.Hosts) == 0 {
			continue
		}
		hosts = append(hosts, entry.Hosts...)
	}
	return hosts
}

func sanitizeSSHKeyName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('_')
		}
	}
	return b.String()
}
