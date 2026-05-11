package key

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

// Supported algorithms for GenerateKeypair.
const (
	AlgoEd25519 = "ed25519"
	AlgoRSA     = "rsa"
)

// minRSABits is the smallest acceptable RSA key size.
const minRSABits = 2048

// GenerateKeypair generates an SSH keypair entirely in memory.
//
// algo:    "ed25519" (default) or "rsa".
// bits:    only used when algo == "rsa"; must be >= 2048.
// comment: appended to the public key (authorized_keys form). If empty,
//
//	the caller is expected to supply a meaningful default; this
//	function does not synthesize one on its own.
//
// Returns the OpenSSH PEM private key, the single-line public key, and the
// SHA256 fingerprint, mirroring ParseAndConvertKey's signature.
func GenerateKeypair(algo string, bits int, comment string) (opensshPriv, publicKey, fingerprint string, err error) {
	if algo == "" {
		algo = AlgoEd25519
	}

	var rawKey interface{}
	switch algo {
	case AlgoEd25519:
		_, priv, genErr := ed25519.GenerateKey(rand.Reader)
		if genErr != nil {
			return "", "", "", fmt.Errorf("generate ed25519 key: %w", genErr)
		}
		// ssh.MarshalPrivateKey for ed25519 expects the *ed25519.PrivateKey-shaped value;
		// pass a pointer so the type switch inside x/crypto/ssh recognises it.
		rawKey = &priv
	case AlgoRSA:
		if bits < minRSABits {
			return "", "", "", fmt.Errorf("rsa key size must be >= %d (got %d)", minRSABits, bits)
		}
		priv, genErr := rsa.GenerateKey(rand.Reader, bits)
		if genErr != nil {
			return "", "", "", fmt.Errorf("generate rsa key: %w", genErr)
		}
		rawKey = priv
	default:
		return "", "", "", fmt.Errorf("unsupported key algorithm: %q (expected %q or %q)", algo, AlgoEd25519, AlgoRSA)
	}

	pemBlock, err := ssh.MarshalPrivateKey(rawKey, "")
	if err != nil {
		return "", "", "", fmt.Errorf("marshal private key to OpenSSH: %w", err)
	}
	opensshPriv = string(pem.EncodeToMemory(pemBlock))

	signer, err := ssh.NewSignerFromKey(rawKey)
	if err != nil {
		return "", "", "", fmt.Errorf("derive signer from generated key: %w", err)
	}
	pub := signer.PublicKey()

	pubLine := strings.TrimRight(string(ssh.MarshalAuthorizedKey(pub)), "\n")
	if c := strings.TrimSpace(comment); c != "" {
		pubLine = pubLine + " " + c
	}
	publicKey = pubLine

	fingerprint = ssh.FingerprintSHA256(pub)
	return opensshPriv, publicKey, fingerprint, nil
}
