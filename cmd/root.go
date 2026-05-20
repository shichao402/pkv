package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shichao402/pkv/internal/tui"
	"github.com/shichao402/pkv/internal/version"
)

var rootCmd = &cobra.Command{
	Use:   "pkv",
	Short: "Personal Key Vault - manage SSH keys and configs from Bitwarden",
	Long: `PKV manages folder-scoped resources from Bitwarden.

Run ` + "`pkv`" + ` without arguments in a terminal to launch the TUI. Use
` + "`PKV_NO_TUI=1 pkv`" + ` or pass a command argument to stay in CLI mode.

Common commands:
  pkv list
  pkv list <folder>
  pkv get <folder> ssh|env|note
  pkv folder add <name>
  pkv add <folder> ssh|env|note
  pkv edit <folder> env
  pkv edit <folder> note <name-or-id>
  pkv remove <folder> ssh <id> [id2]...
  pkv remove <folder> env
  pkv remove <folder> note <id> [id2]...
  pkv clean <folder> ssh|env|note
  pkv unlock`,
	Version:      version.Version,
	SilenceUsage: true,
}

type entryMode int

const (
	entryModeCLI entryMode = iota
	entryModeTUI
)

func Execute(args []string, stdin, stdout, stderr *os.File) error {
	if decideEntryMode(args, isTerminal(stdin), isTerminal(stdout), isTerminal(stderr), os.Getenv) == entryModeTUI {
		return tui.Run(rootCmd.Context())
	}

	rootCmd.SetArgs(args)
	rootCmd.SetIn(stdin)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)
	return rootCmd.Execute()
}

func decideEntryMode(args []string, stdinTTY, stdoutTTY, stderrTTY bool, getenv func(string) string) entryMode {
	if len(args) > 0 {
		return entryModeCLI
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	if getenv("PKV_NO_TUI") == "1" || getenv("TERM") == "dumb" {
		return entryModeCLI
	}
	if !stdinTTY || !stdoutTTY || !stderrTTY {
		return entryModeCLI
	}
	return entryModeTUI
}

func isTerminal(file *os.File) bool {
	if file == nil {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func init() {
	rootCmd.SetVersionTemplate(fmt.Sprintf("pkv %s (commit: %s, built: %s)\n",
		version.Version, version.Commit, version.Date))
}
