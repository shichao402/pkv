package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/shichao402/pkv/internal/bw"
)

type SessionSource string

const (
	SessionSourceMemory SessionSource = "memory"
	SessionSourceEnv    SessionSource = "env"
	SessionSourceFile   SessionSource = "file"
	SessionSourceNone   SessionSource = "none"
)

type ResolvedSession struct {
	Session string
	Source  SessionSource
	Valid   bool
}

var sessionValidator = ValidateBitwardenSession

// SetSessionValidatorForTest overrides session validation in tests.
func SetSessionValidatorForTest(fn func(context.Context, string) error) func() {
	prev := sessionValidator
	if fn == nil {
		sessionValidator = ValidateBitwardenSession
	} else {
		sessionValidator = fn
	}
	return func() { sessionValidator = prev }
}

const needsUnlockHint = `Run in a terminal: pkv unlock
(or: export BW_SESSION="$(pkv unlock)")`

// NeedsUnlockMessage returns guidance when no valid session is available.
func NeedsUnlockMessage() string {
	return needsUnlockHint
}

// ResolveSession resolves a valid Bitwarden session with priority:
// guard memory → BW_SESSION env → ~/.pkv/session file.
func ResolveSession(ctx context.Context, memorySession string) (ResolvedSession, error) {
	candidates := []struct {
		session string
		source  SessionSource
	}{
		{strings.TrimSpace(memorySession), SessionSourceMemory},
		{strings.TrimSpace(os.Getenv("BW_SESSION")), SessionSourceEnv},
	}

	fileSession, err := ReadSession()
	if err != nil {
		return ResolvedSession{Source: SessionSourceNone}, err
	}
	if fileSession != "" {
		candidates = append(candidates, struct {
			session string
			source  SessionSource
		}{fileSession, SessionSourceFile})
	}

	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate.session == "" {
			continue
		}
		if _, ok := seen[candidate.session]; ok {
			continue
		}
		seen[candidate.session] = struct{}{}
		if err := sessionValidator(ctx, candidate.session); err != nil {
			continue
		}
		return ResolvedSession{
			Session: candidate.session,
			Source:  candidate.source,
			Valid:   true,
		}, nil
	}
	return ResolvedSession{Source: SessionSourceNone}, nil
}

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
