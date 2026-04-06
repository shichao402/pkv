package diag

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// Enabled reports whether redacted diagnostic logging is enabled.
// Any truthy PKV_DEBUG value enables logs.
func Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PKV_DEBUG"))) {
	case "1", "true", "yes", "on", "debug":
		return true
	default:
		return false
	}
}

// Printf writes a redacted diagnostic line to stderr when PKV_DEBUG is enabled.
func Printf(format string, args ...any) {
	if !Enabled() {
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "[pkv debug] "+format+"\n", args...)
}

// RedactSecret returns a stable fingerprint for a secret without exposing it.
func RedactSecret(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "<empty>"
	}
	sum := sha256.Sum256([]byte(secret))
	return fmt.Sprintf("<redacted sha256=%s len=%d>", hex.EncodeToString(sum[:6]), len(secret))
}
