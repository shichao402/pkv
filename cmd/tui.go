package cmd

import (
	"github.com/spf13/cobra"

	"github.com/shichao402/pkv/internal/tui"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the experimental interactive TUI",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return tui.Run(commandContext(cmd))
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
