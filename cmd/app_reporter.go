package cmd

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"github.com/shichao402/pkv/internal/app"
)

func cliReporter() app.Reporter {
	return app.TextReporter{Out: os.Stdout, Err: os.Stderr}
}

func commandContext(cmd *cobra.Command) context.Context {
	if cmd == nil || cmd.Context() == nil {
		return context.Background()
	}
	return cmd.Context()
}
