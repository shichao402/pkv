package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shichao402/pkv/internal/bw"
	"github.com/shichao402/pkv/internal/ssh"
	"github.com/shichao402/pkv/internal/state"
)

var sshCmd = &cobra.Command{
	Use:   "ssh <folder> [clean]",
	Short: "Deploy SSH keys from a Bitwarden folder",
	Long: `Deploy all SSH keys from the specified Bitwarden folder to the local machine.

Examples:
  pkv ssh LyraX          Deploy SSH keys from the "LyraX" folder
  pkv ssh LyraX clean    Remove all deployed SSH keys from "LyraX"`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runSSH,
}

func init() {
	rootCmd.AddCommand(sshCmd)
}

func runSSH(_ *cobra.Command, args []string) error {
	folder := args[0]

	if len(args) == 2 {
		return handleCleanCommand(args[1], func() error { return runSSHClean(folder) })
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
