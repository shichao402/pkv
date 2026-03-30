package key

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

// parsePrivateKey parses various formats of private keys (PEM PKCS1/PKCS8/EC, OpenSSH).
// Returns the raw key, ssh.Signer, and any error.
func parsePrivateKey(keyBytes []byte) (interface{}, ssh.Signer, error) {
	// First try ssh.ParsePrivateKey (supports OpenSSH format and PEM)
	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err == nil {
		rawKey, _ := ssh.ParseRawPrivateKey(keyBytes)
		return rawKey, signer, nil
	}

	// Fallback to PEM format parsing
	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return nil, nil, fmt.Errorf("unable to decode PEM data")
	}

	var rawKey interface{}
	switch block.Type {
	case "RSA PRIVATE KEY":
		rawKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, nil, fmt.Errorf("parse RSA PKCS1 key failed: %w", err)
		}
	case "PRIVATE KEY":
		rawKey, err = x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, nil, fmt.Errorf("parse PKCS8 key failed: %w", err)
		}
	case "EC PRIVATE KEY":
		rawKey, err = x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, nil, fmt.Errorf("parse EC key failed: %w", err)
		}
	default:
		return nil, nil, fmt.Errorf("unsupported private key type: %s", block.Type)
	}

	signer, err = ssh.NewSignerFromKey(rawKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create SSH signer failed: %w", err)
	}

	return rawKey, signer, nil
}

// MarshalToOpenSSH converts a raw private key to OpenSSH format string.
func MarshalToOpenSSH(rawKey interface{}) (string, error) {
	pemBlock, err := ssh.MarshalPrivateKey(rawKey, "")
	if err != nil {
		return "", fmt.Errorf("marshal to OpenSSH format failed: %w", err)
	}
	return string(pem.EncodeToMemory(pemBlock)), nil
}

// GenerateFingerprint generates SHA256 fingerprint of a public key.
func GenerateFingerprint(signer ssh.Signer) (string, error) {
	return ssh.FingerprintSHA256(signer.PublicKey()), nil
}

// ParseAndConvertKey parses a private key and converts it to OpenSSH format.
// Returns the OpenSSH formatted private key, public key in ssh-rsa format, and fingerprint.
func ParseAndConvertKey(privateKeyBytes []byte) (opensshPrivateKey, publicKey, fingerprint string, err error) {
	rawKey, signer, err := parsePrivateKey(privateKeyBytes)
	if err != nil {
		return "", "", "", fmt.Errorf("parse private key failed: %w", err)
	}

	// Convert to OpenSSH format if not already
	opensshPrivateKey, err = MarshalToOpenSSH(rawKey)
	if err != nil {
		return "", "", "", fmt.Errorf("convert to OpenSSH format failed: %w", err)
	}

	// Generate public key in ssh-rsa format
	publicKeyData := signer.PublicKey()
	pubKeyBytes := ssh.MarshalAuthorizedKey(publicKeyData)
	publicKeyStr := strings.TrimSpace(string(pubKeyBytes))

	// Generate fingerprint
	fingerprint, err = GenerateFingerprint(signer)
	if err != nil {
		return "", "", "", fmt.Errorf("generate fingerprint failed: %w", err)
	}

	return opensshPrivateKey, publicKeyStr, fingerprint, nil
}

// ValidatePublicKey validates that the provided public key matches a private key.
func ValidatePublicKey(publicKeyStr string, privateKey ssh.Signer) error {
	publicKeyFromPrivate := ssh.MarshalAuthorizedKey(privateKey.PublicKey())
	publicKeyFromPrivateStr := strings.TrimSpace(string(publicKeyFromPrivate))

	// Normalize both keys for comparison (extract the public key part)
	givenParts := parsePublicKeyParts(publicKeyStr)
	privateParts := parsePublicKeyParts(publicKeyFromPrivateStr)

	if len(givenParts) < 2 || len(privateParts) < 2 {
		return fmt.Errorf("invalid public key format")
	}

	// Compare key type and key material
	if givenParts[0] != privateParts[0] || givenParts[1] != privateParts[1] {
		return fmt.Errorf("public key does not match private key")
	}

	return nil
}

// parsePublicKeyParts splits a public key into parts (type, key data, comment).
func parsePublicKeyParts(keyStr string) []string {
	var parts []string
	var current string
	for _, ch := range keyStr {
		if ch == ' ' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// IsValidPrivateKey checks if the given key bytes are a valid private key.
func IsValidPrivateKey(keyBytes []byte) error {
	_, _, err := parsePrivateKey(keyBytes)
	return err
}

// IsValidPublicKey checks if the given string is a valid public key.
func IsValidPublicKey(pubKeyStr string) error {
	_, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pubKeyStr))
	return err
}

// GetPublicKeyFromPrivate extracts the public key from a private key.
func GetPublicKeyFromPrivate(privateKeyBytes []byte) (string, error) {
	_, signer, err := parsePrivateKey(privateKeyBytes)
	if err != nil {
		return "", err
	}

	pubKeyBytes := ssh.MarshalAuthorizedKey(signer.PublicKey())
	return strings.TrimSpace(string(pubKeyBytes)), nil
}
