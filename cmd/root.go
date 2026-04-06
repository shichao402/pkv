package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shichao402/pkv/internal/version"
)

var rootCmd = &cobra.Command{
	Use:   "pkv",
	Short: "Personal Key Vault - manage SSH keys and configs from Bitwarden",
	Long: `PKV manages folder-scoped resources from Bitwarden.

Run ` + "`pkv`" + ` without arguments to enter interactive mode. The process keeps the
Bitwarden session in memory, so repeated commands in the same shell do not ask
for the master password again.

Common commands:
  pkv list
  pkv list <folder>
  pkv get <folder> ssh|env|note
  pkv add <folder> ssh|env|note
  pkv edit <folder> env
  pkv edit <folder> note <name-or-id>
  pkv remove <folder> ssh <id> [id2]...
  pkv remove <folder> env
  pkv remove <folder> note <id> [id2]...
  pkv clean <folder> ssh|env|note`,
	Version:      version.Version,
	SilenceUsage: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Args = cobra.NoArgs
	rootCmd.RunE = runShell
	rootCmd.SetVersionTemplate(fmt.Sprintf("pkv %s (commit: %s, built: %s)\n",
		version.Version, version.Commit, version.Date))
}
