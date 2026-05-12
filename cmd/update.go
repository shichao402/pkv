package cmd

import (
	"github.com/spf13/cobra"

	"github.com/shichao402/pkv/internal/app"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update pkv to the latest version",
	RunE:  runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, _ []string) error {
	_, err := app.Update(commandContext(cmd), app.UpdateParams{}, cliReporter())
	return err
}
