package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/shichao402/pkv/internal/bw"
)

type bitwardenSessionContextKey struct{}

type bitwardenSessionClient interface {
	EnsureUnlocked() (string, error)
}

func WithBitwardenSession(ctx context.Context, session string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	session = strings.TrimSpace(session)
	if session == "" {
		return ctx
	}
	return context.WithValue(ctx, bitwardenSessionContextKey{}, session)
}

func BitwardenSessionFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	session, ok := ctx.Value(bitwardenSessionContextKey{}).(string)
	session = strings.TrimSpace(session)
	return session, ok && session != ""
}

func ensureBitwardenSession(ctx context.Context, client bitwardenSessionClient, r Reporter) (string, error) {
	if session, ok := BitwardenSessionFromContext(ctx); ok {
		return session, nil
	}
	r = reporterOrNoop(r)
	r.Info("Authenticating with Bitwarden...")
	session, err := client.EnsureUnlocked()
	if err != nil {
		return "", err
	}
	return session, nil
}

func ValidateBitwardenSession(ctx context.Context, session string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	client := bw.NewClient()
	if _, err := client.ListFolders(strings.TrimSpace(session)); err != nil {
		return fmt.Errorf("validate BW_SESSION: %w", err)
	}
	return ctx.Err()
}
