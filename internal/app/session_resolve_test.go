package app

import (
	"context"
	"fmt"
	"testing"
)

func TestResolveSessionPriority(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BW_SESSION", "env-session")

	restore := SetSessionValidatorForTest(func(_ context.Context, session string) error {
		switch session {
		case "memory-session", "env-session", "file-session":
			return nil
		default:
			return fmt.Errorf("invalid session")
		}
	})
	defer restore()

	if err := WriteSession("file-session"); err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveSession(t.Context(), "memory-session")
	if err != nil {
		t.Fatalf("ResolveSession() error = %v", err)
	}
	if !resolved.Valid || resolved.Session != "memory-session" || resolved.Source != SessionSourceMemory {
		t.Fatalf("ResolveSession() = %+v, want memory-session from memory", resolved)
	}

	resolved, err = ResolveSession(t.Context(), "")
	if err != nil {
		t.Fatalf("ResolveSession() error = %v", err)
	}
	if !resolved.Valid || resolved.Session != "env-session" || resolved.Source != SessionSourceEnv {
		t.Fatalf("ResolveSession() = %+v, want env-session from env", resolved)
	}

	t.Setenv("BW_SESSION", "")
	resolved, err = ResolveSession(t.Context(), "")
	if err != nil {
		t.Fatalf("ResolveSession() error = %v", err)
	}
	if !resolved.Valid || resolved.Session != "file-session" || resolved.Source != SessionSourceFile {
		t.Fatalf("ResolveSession() = %+v, want file-session from file", resolved)
	}
}

func TestResolveSessionNone(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BW_SESSION", "")

	restore := SetSessionValidatorForTest(func(context.Context, string) error {
		return fmt.Errorf("invalid")
	})
	defer restore()

	resolved, err := ResolveSession(t.Context(), "")
	if err != nil {
		t.Fatalf("ResolveSession() error = %v", err)
	}
	if resolved.Valid || resolved.Source != SessionSourceNone {
		t.Fatalf("ResolveSession() = %+v, want invalid none", resolved)
	}
}
