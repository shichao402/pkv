package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shichao402/pkv/internal/bw"
	"github.com/shichao402/pkv/internal/key"
	"github.com/shichao402/pkv/internal/ssh"
	"github.com/shichao402/pkv/internal/state"
)

var sshCmd = &cobra.Command{
	Use:   "ssh <folder> [add|list|remove|clean]",
	Short: "Deploy and manage SSH keys in a Bitwarden folder",
	Long: `Deploy, list, add, and remove SSH keys in a Bitwarden folder.

Examples:
  pkv ssh LyraX              Deploy SSH keys from the "LyraX" folder
  pkv ssh LyraX list         List SSH keys in the folder
  pkv ssh LyraX add --priv ~/.ssh/id_rsa --name "my-key"   Add a local key
  pkv ssh LyraX remove <id>  Remove a key from Bitwarden
  pkv ssh LyraX clean        Remove all locally deployed SSH keys`,
	Args:               cobra.MinimumNArgs(1),
	RunE:               runSSH,
	DisableFlagParsing: false,
}

var (
	sshAddPrivFlag string
	sshAddPubFlag  string
	sshAddNameFlag string
)

func init() {
	rootCmd.AddCommand(sshCmd)
	sshCmd.Flags().StringVar(&sshAddPrivFlag, "priv", "", "Private key file path (used with 'add')")
	sshCmd.Flags().StringVar(&sshAddPubFlag, "pub", "", "Public key, ssh-rsa AAAA... format (used with 'add')")
	sshCmd.Flags().StringVar(&sshAddNameFlag, "name", "", "Key name in Bitwarden (used with 'add')")
}

func runSSH(_ *cobra.Command, args []string) error {
	folder := args[0]

	if len(args) >= 2 {
		switch args[1] {
		case "clean":
			return runSSHClean(folder)
		case "add":
			return runSSHAdd(folder)
		case "list":
			return runSSHList(folder)
		case "remove":
			if len(args) < 3 {
				return fmt.Errorf("usage: pkv ssh <folder> remove <id> [id2] [id3]...")
			}
			return runSSHRemove(folder, args[2:])
		default:
			return fmt.Errorf("unknown option: %s (expected 'add', 'list', 'remove', or 'clean')", args[1])
		}
	}

	return runSSHDeploy(folder)
}

func runSSHDeploy(folder string) error {
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
	if len(sshKeys) == 0 {
		fmt.Println("No SSH keys found in folder.")
		return nil
	}

	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state failed: %w", err)
	}

	deployer := ssh.NewDeployer(st)
	deployed := 0
	var allHosts []string
	for _, key := range sshKeys {
		fmt.Printf("  Deploying '%s'...\n", key.Name)
		if err := deployer.Deploy(key); err != nil {
			fmt.Fprintf(os.Stderr, "  Failed to deploy '%s': %v\n", key.Name, err)
			continue
		}
		allHosts = append(allHosts, key.GetHosts()...)
		deployed++
	}

	// Scan and add known_hosts for all deployed hosts
	if len(allHosts) > 0 {
		fmt.Println("Scanning host keys for known_hosts...")
		if err := deployer.DeployKnownHosts(allHosts); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: known_hosts update failed: %v\n", err)
		}
	}

	if err := st.Save(); err != nil {
		return fmt.Errorf("save state failed: %w", err)
	}

	fmt.Printf("Deployed %d SSH key(s).\n", deployed)
	return nil
}

func runSSHClean(folder string) error {
	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state failed: %w", err)
	}

	// Filter keys belonging to this folder (by key name prefix or all if not tracked by folder)
	if len(st.SSHKeys) == 0 {
		fmt.Println("No SSH keys to clean.")
		return nil
	}

	deployer := ssh.NewDeployer(st)
	cleaned := 0
	for _, entry := range st.SSHKeys {
		fmt.Printf("  Removing '%s'...\n", entry.KeyName)
		if err := deployer.Remove(entry); err != nil {
			fmt.Fprintf(os.Stderr, "  Failed to remove '%s': %v\n", entry.KeyName, err)
			continue
		}
		cleaned++
	}

	// Remove PKV managed entries from known_hosts
	fmt.Println("  Cleaning known_hosts...")
	if err := deployer.RemoveAllKnownHosts(); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: known_hosts cleanup failed: %v\n", err)
	}

	st.SSHKeys = nil
	if err := st.Save(); err != nil {
		return fmt.Errorf("save state failed: %w", err)
	}

	fmt.Printf("Cleaned %d SSH key(s).\n", cleaned)
	return nil
}

func runSSHAdd(folder string) error {
	cfg := &key.InputConfig{
		PrivatePath: sshAddPrivFlag,
		PublicKey:   sshAddPubFlag,
		KeyName:     sshAddNameFlag,
		Folder:      folder,
	}

	// Expand ~ in --priv path
	if strings.HasPrefix(cfg.PrivatePath, "~") {
		home, _ := os.UserHomeDir()
		cfg.PrivatePath = filepath.Join(home, cfg.PrivatePath[1:])
	}

	fmt.Printf("Adding SSH key to Bitwarden folder '%s'...\n", folder)
	if err := key.InteractiveInput(cfg); err != nil {
		return fmt.Errorf("input failed: %w", err)
	}

	// Read private key
	fmt.Printf("\nReading private key: %s\n", cfg.PrivatePath)
	privateKeyBytes, err := os.ReadFile(cfg.PrivatePath)
	if err != nil {
		return fmt.Errorf("read private key failed: %w", err)
	}

	// Parse and convert
	fmt.Println("Parsing and converting key...")
	opensshKey, publicKey, fingerprint, err := key.ParseAndConvertKey(privateKeyBytes)
	if err != nil {
		return fmt.Errorf("parse key failed: %w", err)
	}

	// Authenticate with Bitwarden using existing bw.Client
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

	// Resolve folder ID
	fmt.Printf("Looking up folder '%s'...\n", folder)
	folderID, err := client.GetFolderID(session, folder)
	if err != nil {
		return fmt.Errorf("folder lookup failed: %w", err)
	}

	// Confirm
	confirm, err := key.ConfirmAndCreate(cfg, fingerprint)
	if err != nil {
		return fmt.Errorf("confirmation failed: %w", err)
	}
	if !confirm {
		fmt.Println("Cancelled.")
		return nil
	}

	// Create SSH key in Bitwarden
	fmt.Println("Creating SSH key in Bitwarden...")
	output, err := key.CreateBWSSHKey(session, cfg.KeyName, folderID, opensshKey, publicKey, fingerprint)
	if err != nil {
		return fmt.Errorf("create SSH key failed: %w", err)
	}

	// Record in state
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
}

func runSSHList(folder string) error {
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
	if len(sshKeys) == 0 {
		fmt.Printf("No SSH keys found in folder '%s'.\n", folder)
		return nil
	}

	// Calculate column widths
	nameWidth := 4 // "Name"
	fpWidth := 11  // "Fingerprint"
	for _, k := range sshKeys {
		if len(k.Name) > nameWidth {
			nameWidth = len(k.Name)
		}
		fp := ""
		if k.SSHKey != nil {
			fp = truncateFingerprint(k.SSHKey.KeyFingerprint)
		}
		if len(fp) > fpWidth {
			fpWidth = len(fp)
		}
	}

	// Print header
	fmt.Printf("\nSSH Keys in folder '%s':\n\n", folder)
	fmt.Printf("%-36s  %-*s  %-*s  %s\n", "ID", nameWidth, "Name", fpWidth, "Fingerprint", "Hosts")
	fmt.Printf("%-36s  %-*s  %-*s  %s\n", "----", nameWidth, "----", fpWidth, "-----------", "-----")

	for _, k := range sshKeys {
		fp := "-"
		if k.SSHKey != nil && k.SSHKey.KeyFingerprint != "" {
			fp = truncateFingerprint(k.SSHKey.KeyFingerprint)
		}
		hosts := formatHosts(k.GetHosts(), 2)
		fmt.Printf("%-36s  %-*s  %-*s  %s\n", k.ID, nameWidth, k.Name, fpWidth, fp, hosts)
	}

	fmt.Printf("\n%d SSH key(s) found.\n", len(sshKeys))
	return nil
}

func runSSHRemove(folder string, keyIDs []string) error {
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

	// Build lookup map
	keyMap := make(map[string]string) // id -> name
	for _, k := range sshKeys {
		keyMap[k.ID] = k.Name
	}

	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state failed: %w", err)
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

		st.RemoveStoredSSHKey(id)
		fmt.Printf("  Removed '%s' (%s)\n", name, id)
		removed++
	}

	if err := st.Save(); err != nil {
		return fmt.Errorf("save state failed: %w", err)
	}

	fmt.Printf("Removed %d SSH key(s).\n", removed)
	return nil
}

// truncateFingerprint truncates a fingerprint for display (e.g. "SHA256:abc...xyz").
func truncateFingerprint(fp string) string {
	if len(fp) <= 30 {
		return fp
	}
	return fp[:25] + "..."
}

// formatHosts formats a host list for display, showing up to maxHosts entries.
func formatHosts(hosts []string, maxHosts int) string {
	if len(hosts) == 0 {
		return "-"
	}
	if len(hosts) <= maxHosts {
		return strings.Join(hosts, ", ")
	}
	return strings.Join(hosts[:maxHosts], ", ") + fmt.Sprintf(" (+%d)", len(hosts)-maxHosts)
}
