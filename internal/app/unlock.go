package app

import (
	"context"
	"fmt"

	"github.com/shichao402/pkv/internal/bw"
)

type UnlockParams struct{}

type UnlockResult struct {
	Session string
}

func Unlock(ctx context.Context, _ UnlockParams, r Reporter) (UnlockResult, error) {
	r = reporterOrNoop(r)
	client := bw.NewClient()

	r.Info("Authenticating with Bitwarden...")
	session, err := client.EnsureUnlocked()
	if err != nil {
		return UnlockResult{}, fmt.Errorf("authentication failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return UnlockResult{}, err
	}
	if err := WriteSession(session); err != nil {
		return UnlockResult{}, fmt.Errorf("persist session: %w", err)
	}
	return UnlockResult{Session: session}, nil
}
