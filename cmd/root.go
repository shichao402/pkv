package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shichao402/pkv/internal/version"
)

var rootCmd = &cobra.Command{
	Use:          "pkv",
	Short:        "Personal Key Vault - manage SSH keys and configs from Bitwarden",
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
	rootCmd.SetVersionTemplate(fmt.Sprintf("pkv %s (commit: %s, built: %s)\n",
		version.Version, version.Commit, version.Date))
}
