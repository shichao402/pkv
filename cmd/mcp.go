package cmd

import (
	"github.com/spf13/cobra"

	pkvmcp "github.com/shichao402/pkv/internal/mcp"
)

var mcpCmd = &cobra.Command{
	Use:    "mcp",
	Short:  "Run PKV as an MCP server over stdio",
	Hidden: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		_ = cmd
		return pkvmcp.ServeStdio()
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}
