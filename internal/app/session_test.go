package app

import "testing"

func TestEnsureBitwardenSessionUsesContextSession(t *testing.T) {
	client := &fakeSessionClient{session: "fresh-session"}
	ctx := WithBitwardenSession(t.Context(), "startup-session")

	session, err := ensureBitwardenSession(ctx, client, nil)
	if err != nil {
		t.Fatalf("ensureBitwardenSession() error = %v, want nil", err)
	}
	if session != "startup-session" {
		t.Fatalf("session = %q, want startup-session", session)
	}
	if client.ensureUnlockedCalls != 0 {
		t.Fatalf("EnsureUnlocked calls = %d, want 0", client.ensureUnlockedCalls)
	}
}

func TestEnsureBitwardenSessionAuthenticatesWithoutContextSession(t *testing.T) {
	client := &fakeSessionClient{session: "fresh-session"}

	session, err := ensureBitwardenSession(t.Context(), client, nil)
	if err != nil {
		t.Fatalf("ensureBitwardenSession() error = %v, want nil", err)
	}
	if session != "fresh-session" {
		t.Fatalf("session = %q, want fresh-session", session)
	}
	if client.ensureUnlockedCalls != 1 {
		t.Fatalf("EnsureUnlocked calls = %d, want 1", client.ensureUnlockedCalls)
	}
}

type fakeSessionClient struct {
	session             string
	ensureUnlockedCalls int
}

func (c *fakeSessionClient) EnsureUnlocked() (string, error) {
	c.ensureUnlockedCalls++
	return c.session, nil
}
