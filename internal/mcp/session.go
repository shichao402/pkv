package mcp

import (
	"context"
	"fmt"

	"github.com/shichao402/pkv/internal/app"
	"github.com/shichao402/pkv/internal/unlock"
)

func (s *Server) ensureSessionForMCP(ctx context.Context) error {
	if !s.guard.Status().SessionMissing {
		return nil
	}

	if s.guard.ResolveSessionFromSources(ctx) {
		return nil
	}

	session, err := unlock.Run(ctx, func(ctx context.Context, password string) (string, error) {
		result, err := app.UnlockWithPassword(ctx, password)
		if err != nil {
			return "", err
		}
		return result.Session, nil
	}, unlock.Options{})
	if err != nil {
		return fmt.Errorf("web unlock failed: %w", err)
	}

	s.guard.SetSession(session)
	return nil
}
