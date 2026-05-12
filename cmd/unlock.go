package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shichao402/pkv/internal/app"
)

var (
	unlockExportFlag bool
	// unlockQuiet suppresses stdout printing of the session. Used by the
	// interactive REPL so the session token never appears on screen.
	unlockQuiet bool
)

var unlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: "Unlock Bitwarden and print the session for shell use",
	Long: `Unlock the Bitwarden vault and print the session key so it can be reused
across subsequent PKV commands.

By default the session is written as a bare string to stdout, so shell
substitution works cleanly:

  export BW_SESSION="$(pkv unlock)"

With --export, the output is a ready-to-eval shell statement:

  eval "$(pkv unlock --export)"

If BW_SESSION is already set and still valid, it is reused and printed
without prompting for the master password. All diagnostic messages go to
stderr so stdout remains safe for command substitution.

This command never accepts the master password itself. Interaction with
'bw unlock' happens over the terminal, the same way it would if you ran
'bw unlock --raw' directly.`,
	Args: cobra.NoArgs,
	RunE: runUnlock,
}

func init() {
	unlockCmd.Flags().BoolVar(&unlockExportFlag, "export", false,
		`Print a shell 'export BW_SESSION="..."' statement instead of the raw session`)
	rootCmd.AddCommand(unlockCmd)
}

func runUnlock(cmd *cobra.Command, _ []string) error {
	reporter := app.TextReporter{Out: os.Stderr, Err: os.Stderr}
	result, err := app.Unlock(commandContext(cmd), app.UnlockParams{}, reporter)
	if err != nil {
		return err
	}

	if unlockQuiet {
		fmt.Fprintln(os.Stderr, "Vault unlocked.")
		return nil
	}

	if unlockExportFlag {
		fmt.Printf("export BW_SESSION=%q\n", result.Session)
	} else {
		fmt.Println(result.Session)
	}
	return nil
}
