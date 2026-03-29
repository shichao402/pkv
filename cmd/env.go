package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/shichao402/pkv/internal/bw"
	"github.com/shichao402/pkv/internal/env"
	"github.com/shichao402/pkv/internal/state"
	"github.com/spf13/cobra"
)

var envCmd = &cobra.Command{
	Use:   "env <folder> [clean]",
	Short: "Deploy environment variables from a Bitwarden folder",
	Long: `Deploy environment variables from Secure Notes in the specified Bitwarden folder.

Each Secure Note must have a custom field "pkv_type" set to "env" to be recognized.
Notes without this field will be skipped with a warning.

The note content should contain KEY=VALUE pairs (one per line).
Supports: KEY=VALUE, export KEY=VALUE, # comments, quoted values.

On Windows, variables are set as persistent User environment variables.
On Linux/macOS, variables are written to ~/.pkv/env.sh and sourced from shell rc files.

Examples:
  pkv env github          Deploy env vars from the "github" folder
  pkv env github clean    Remove deployed env vars from "github"`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runEnv,
}

func init() {
	rootCmd.AddCommand(envCmd)
}

func runEnv(_ *cobra.Command, args []string) error {
	folder := args[0]

	if len(args) == 2 {
		if args[1] != "clean" {
			return fmt.Errorf("unknown option: %s (expected 'clean')", args[1])
		}
		return runEnvClean()
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

	deployer := env.NewDeployer(st)
	totalVars := 0
	for _, note := range notes {
		fmt.Printf("  Deploying '%s'...\n", note.Name)
		vars, err := deployer.Deploy(note)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Failed to deploy '%s': %v\n", note.Name, err)
			continue
		}
		for _, v := range vars {
			fmt.Printf("    ✓ %s\n", v.Key)
		}
		totalVars += len(vars)
	}

	if err := st.Save(); err != nil {
		return fmt.Errorf("save state failed: %w", err)
	}

	fmt.Printf("Deployed %d variable(s). Open a new terminal to use them.\n", totalVars)
	return nil
}

func runEnvClean() error {
	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state failed: %w", err)
	}

	if len(st.Envs) == 0 {
		fmt.Println("No environment variables to clean.")
		return nil
	}

	deployer := env.NewDeployer(st)
	cleaned := 0
	for _, entry := range st.Envs {
		fmt.Printf("  Removing '%s' (%s)...\n", entry.Name, strings.Join(entry.Keys, ", "))
		if err := deployer.Remove(entry); err != nil {
			fmt.Fprintf(os.Stderr, "  Failed to remove '%s': %v\n", entry.Name, err)
			continue
		}
		cleaned++
	}

	st.Envs = nil
	if err := st.Save(); err != nil {
		return fmt.Errorf("save state failed: %w", err)
	}

	fmt.Printf("Cleaned %d env group(s). Restart terminal to apply.\n", cleaned)
	return nil
}
