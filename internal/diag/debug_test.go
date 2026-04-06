package diag

import (
	"strings"
	"testing"
)

func TestEnabled(t *testing.T) {
	t.Setenv("PKV_DEBUG", "1")
	if !Enabled() {
		t.Fatal("Enabled() = false, want true")
	}

	t.Setenv("PKV_DEBUG", "off")
	if Enabled() {
		t.Fatal("Enabled() = true, want false")
	}
}

func TestRedactSecret(t *testing.T) {
	if got := RedactSecret(""); got != "<empty>" {
		t.Fatalf("RedactSecret(empty) = %q", got)
	}

	got := RedactSecret("super-secret")
	if strings.Contains(got, "super-secret") {
		t.Fatalf("RedactSecret leaked input: %q", got)
	}
	if !strings.Contains(got, "len=12") {
		t.Fatalf("RedactSecret() = %q, want length metadata", got)
	}
	if !strings.Contains(got, "sha256=") {
		t.Fatalf("RedactSecret() = %q, want hash metadata", got)
	}

	if got != RedactSecret("super-secret") {
		t.Fatalf("RedactSecret() should be deterministic, got %q", got)
	}
}
