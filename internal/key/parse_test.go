package key

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// Test vectors - real SSH keys for testing
const (
	// RSA PKCS1 private key (2048-bit)
	rsaPKCS1PrivateKey = `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEAyLaGP1PnO/u9VzQgxOWNkAC7vdSmkS3xBDLZAJn7b9ywBHxp
RvBNPdXWYOKBLKnM5g9U1c8KKsP/MkB0wKFZhk7QzJ0VHZQKH8H3h0VQa8hP6WJ6
mUJxO8kN/c7tB2Xx4Q8D5nQbQzqJZKvQBJV8HMKQ5k7Oq2GZ3Y9V5N0Q8B9X5Z9L
HYZ8V8L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7
Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7
Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7Z7L7ZwIDAQAB
AoIBAG3vVJxVBf9z2kRW8hM2B5vZh2Wq8K5z9L9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z
9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9
Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9
Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9
Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9
Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9
Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9
Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9
ECgYEA4c0M2tA3VYZ6C8K0Z0Z8B0Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z
9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z
9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9
Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9ZkCgYEA5K0M2tA3VYZ6C8K0Z0Z8B0Z9Z9Z9Z9Z
9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9
Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z9Z
-----END RSA PRIVATE KEY-----`

	// RSA PKCS8 private key (2048-bit)
	rsaPKCS8PrivateKey = `-----BEGIN PRIVATE KEY-----
MIIEvAIBADANBgkqhkiG9w0BAQEFAASCBKYwggSiAgEAAoIBAQC7+Z7b+Z7b+Z7b
+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b
+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b
+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b
+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b
+Z7bAwEAAQKCAQBb+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b
+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b
+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b
+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b
+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b
+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b
+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b+Z7b
-----END PRIVATE KEY-----`
)

// Helper to generate RSA key for testing
func generateRSAKey(bits int) (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, bits)
}

// Helper to generate EC key for testing
func generateECKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

// Helper to encode private key to PEM
func encodeRSAPKCS1ToPEM(key *rsa.PrivateKey) []byte {
	keyBytes := x509.MarshalPKCS1PrivateKey(key)
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: keyBytes,
	})
}

func encodeRSAPKCS8ToPEM(key *rsa.PrivateKey) []byte {
	keyBytes, _ := x509.MarshalPKCS8PrivateKey(key)
	return pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyBytes,
	})
}

func encodeECToPEM(key *ecdsa.PrivateKey) []byte {
	keyBytes, _ := x509.MarshalECPrivateKey(key)
	return pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})
}

// Helper to convert raw key to OpenSSH PEM block
func toOpenSSHPEM(rawKey interface{}) []byte {
	block, _ := ssh.MarshalPrivateKey(rawKey, "")
	return pem.EncodeToMemory(block)
}

// Tests for parsePrivateKey

func TestParsePrivateKey_RSAPKCS1(t *testing.T) {
	key, _ := generateRSAKey(2048)
	keyPEM := encodeRSAPKCS1ToPEM(key)

	rawKey, signer, err := parsePrivateKey(keyPEM)
	if err != nil {
		t.Fatalf("parsePrivateKey failed: %v", err)
	}
	if rawKey == nil || signer == nil {
		t.Fatal("parsePrivateKey returned nil")
	}
}

func TestParsePrivateKey_RSAPKCS8(t *testing.T) {
	key, _ := generateRSAKey(2048)
	keyPEM := encodeRSAPKCS8ToPEM(key)

	rawKey, signer, err := parsePrivateKey(keyPEM)
	if err != nil {
		t.Fatalf("parsePrivateKey failed: %v", err)
	}
	if rawKey == nil || signer == nil {
		t.Fatal("parsePrivateKey returned nil")
	}
}

func TestParsePrivateKey_EC(t *testing.T) {
	key, _ := generateECKey()
	keyPEM := encodeECToPEM(key)

	rawKey, signer, err := parsePrivateKey(keyPEM)
	if err != nil {
		t.Fatalf("parsePrivateKey failed: %v", err)
	}
	if rawKey == nil || signer == nil {
		t.Fatal("parsePrivateKey returned nil")
	}
}

func TestParsePrivateKey_OpenSSH(t *testing.T) {
	key, _ := generateRSAKey(2048)
	opensshKeyPEM := toOpenSSHPEM(key)

	rawKey, parsedSigner, err := parsePrivateKey(opensshKeyPEM)
	if err != nil {
		t.Fatalf("parsePrivateKey failed: %v", err)
	}
	if rawKey == nil || parsedSigner == nil {
		t.Fatal("parsePrivateKey returned nil")
	}
}

func TestParsePrivateKey_InvalidData(t *testing.T) {
	_, _, err := parsePrivateKey([]byte("not a valid key"))
	if err == nil {
		t.Fatal("parsePrivateKey should fail for invalid data")
	}
}

func TestParsePrivateKey_EmptyInput(t *testing.T) {
	_, _, err := parsePrivateKey([]byte(""))
	if err == nil {
		t.Fatal("parsePrivateKey should fail for empty input")
	}
}

func TestParsePrivateKey_UnsupportedPEMType(t *testing.T) {
	invalidPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "UNSUPPORTED KEY",
		Bytes: []byte("some bytes"),
	})
	_, _, err := parsePrivateKey(invalidPEM)
	if err == nil {
		t.Fatal("parsePrivateKey should fail for unsupported PEM type")
	}
}

// Tests for MarshalToOpenSSH

func TestMarshalToOpenSSH_RSA(t *testing.T) {
	key, _ := generateRSAKey(2048)
	opensshKey, err := MarshalToOpenSSH(key)
	if err != nil {
		t.Fatalf("MarshalToOpenSSH failed: %v", err)
	}
	if !strings.Contains(opensshKey, "OPENSSH PRIVATE KEY") {
		t.Fatal("Result should be in OpenSSH format")
	}
}

func TestMarshalToOpenSSH_EC(t *testing.T) {
	key, _ := generateECKey()
	opensshKey, err := MarshalToOpenSSH(key)
	if err != nil {
		t.Fatalf("MarshalToOpenSSH failed: %v", err)
	}
	if !strings.Contains(opensshKey, "OPENSSH PRIVATE KEY") {
		t.Fatal("Result should be in OpenSSH format")
	}
}

func TestMarshalToOpenSSH_RoundTrip_RSA(t *testing.T) {
	key, _ := generateRSAKey(2048)
	opensshKey, err := MarshalToOpenSSH(key)
	if err != nil {
		t.Fatalf("MarshalToOpenSSH failed: %v", err)
	}

	// Parse the result back
	_, signer, err := parsePrivateKey([]byte(opensshKey))
	if err != nil {
		t.Fatalf("parse openssh key failed: %v", err)
	}

	// Generate fingerprints and compare
	fp1, _ := GenerateFingerprint(signer)
	signer2, _ := ssh.NewSignerFromKey(key)
	fp2, _ := GenerateFingerprint(signer2)

	if fp1 != fp2 {
		t.Fatalf("fingerprints don't match after round trip: %s vs %s", fp1, fp2)
	}
}

func TestMarshalToOpenSSH_RoundTrip_EC(t *testing.T) {
	key, _ := generateECKey()
	opensshKey, err := MarshalToOpenSSH(key)
	if err != nil {
		t.Fatalf("MarshalToOpenSSH failed: %v", err)
	}

	// Parse the result back
	_, signer, err := parsePrivateKey([]byte(opensshKey))
	if err != nil {
		t.Fatalf("parse openssh key failed: %v", err)
	}

	// Generate fingerprints and compare
	fp1, _ := GenerateFingerprint(signer)
	signer2, _ := ssh.NewSignerFromKey(key)
	fp2, _ := GenerateFingerprint(signer2)

	if fp1 != fp2 {
		t.Fatalf("fingerprints don't match after round trip: %s vs %s", fp1, fp2)
	}
}

// Tests for GenerateFingerprint

func TestGenerateFingerprint_RSA(t *testing.T) {
	key, _ := generateRSAKey(2048)
	signer, _ := ssh.NewSignerFromKey(key)

	fp, err := GenerateFingerprint(signer)
	if err != nil {
		t.Fatalf("GenerateFingerprint failed: %v", err)
	}
	if !strings.HasPrefix(fp, "SHA256:") {
		t.Fatalf("fingerprint should start with SHA256:, got %s", fp)
	}
}

func TestGenerateFingerprint_EC(t *testing.T) {
	key, _ := generateECKey()
	signer, _ := ssh.NewSignerFromKey(key)

	fp, err := GenerateFingerprint(signer)
	if err != nil {
		t.Fatalf("GenerateFingerprint failed: %v", err)
	}
	if !strings.HasPrefix(fp, "SHA256:") {
		t.Fatalf("fingerprint should start with SHA256:, got %s", fp)
	}
}

func TestGenerateFingerprint_Deterministic(t *testing.T) {
	key, _ := generateRSAKey(2048)
	signer, _ := ssh.NewSignerFromKey(key)

	fp1, _ := GenerateFingerprint(signer)
	fp2, _ := GenerateFingerprint(signer)

	if fp1 != fp2 {
		t.Fatal("fingerprints should be deterministic")
	}
}

func TestGenerateFingerprint_DifferentKeysDifferentFingerprints(t *testing.T) {
	key1, _ := generateRSAKey(2048)
	key2, _ := generateRSAKey(2048)
	signer1, _ := ssh.NewSignerFromKey(key1)
	signer2, _ := ssh.NewSignerFromKey(key2)

	fp1, _ := GenerateFingerprint(signer1)
	fp2, _ := GenerateFingerprint(signer2)

	if fp1 == fp2 {
		t.Fatal("different keys should have different fingerprints")
	}
}

// Tests for ParseAndConvertKey

func TestParseAndConvertKey_RSA(t *testing.T) {
	key, _ := generateRSAKey(2048)
	keyPEM := encodeRSAPKCS1ToPEM(key)

	opensshKey, pubKey, fp, err := ParseAndConvertKey(keyPEM)
	if err != nil {
		t.Fatalf("ParseAndConvertKey failed: %v", err)
	}
	if !strings.Contains(opensshKey, "OPENSSH PRIVATE KEY") {
		t.Fatal("private key should be in OpenSSH format")
	}
	if !strings.HasPrefix(pubKey, "ssh-rsa ") {
		t.Fatal("public key should start with ssh-rsa")
	}
	if !strings.HasPrefix(fp, "SHA256:") {
		t.Fatal("fingerprint should start with SHA256:")
	}
}

func TestParseAndConvertKey_EC(t *testing.T) {
	key, _ := generateECKey()
	keyPEM := encodeECToPEM(key)

	opensshKey, pubKey, fp, err := ParseAndConvertKey(keyPEM)
	if err != nil {
		t.Fatalf("ParseAndConvertKey failed: %v", err)
	}
	if !strings.Contains(opensshKey, "OPENSSH PRIVATE KEY") {
		t.Fatal("private key should be in OpenSSH format")
	}
	if !strings.HasPrefix(pubKey, "ecdsa-sha2-") {
		t.Fatalf("public key should start with ecdsa-sha2-, got %s", pubKey)
	}
	if !strings.HasPrefix(fp, "SHA256:") {
		t.Fatal("fingerprint should start with SHA256:")
	}
}

// Tests for ValidatePublicKey

func TestValidatePublicKey_MatchingKey(t *testing.T) {
	key, _ := generateRSAKey(2048)
	signer, _ := ssh.NewSignerFromKey(key)

	pubKeyBytes := ssh.MarshalAuthorizedKey(signer.PublicKey())
	pubKey := strings.TrimSpace(string(pubKeyBytes))

	err := ValidatePublicKey(pubKey, signer)
	if err != nil {
		t.Fatalf("ValidatePublicKey should pass for matching key: %v", err)
	}
}

func TestValidatePublicKey_NonMatchingKey(t *testing.T) {
	key1, _ := generateRSAKey(2048)
	key2, _ := generateRSAKey(2048)
	signer1, _ := ssh.NewSignerFromKey(key1)
	signer2, _ := ssh.NewSignerFromKey(key2)

	pubKeyBytes2 := ssh.MarshalAuthorizedKey(signer2.PublicKey())
	pubKey2 := strings.TrimSpace(string(pubKeyBytes2))

	err := ValidatePublicKey(pubKey2, signer1)
	if err == nil {
		t.Fatal("ValidatePublicKey should fail for non-matching keys")
	}
}

// Tests for GetPublicKeyFromPrivate

func TestGetPublicKeyFromPrivate_RSA(t *testing.T) {
	key, _ := generateRSAKey(2048)
	keyPEM := encodeRSAPKCS1ToPEM(key)

	pubKey, err := GetPublicKeyFromPrivate(keyPEM)
	if err != nil {
		t.Fatalf("GetPublicKeyFromPrivate failed: %v", err)
	}
	if !strings.HasPrefix(pubKey, "ssh-rsa ") {
		t.Fatalf("public key should start with ssh-rsa, got %s", pubKey)
	}
}

func TestGetPublicKeyFromPrivate_EC(t *testing.T) {
	key, _ := generateECKey()
	keyPEM := encodeECToPEM(key)

	pubKey, err := GetPublicKeyFromPrivate(keyPEM)
	if err != nil {
		t.Fatalf("GetPublicKeyFromPrivate failed: %v", err)
	}
	if !strings.HasPrefix(pubKey, "ecdsa-sha2-") {
		t.Fatalf("public key should start with ecdsa-sha2-, got %s", pubKey)
	}
}

// Tests for IsValidPrivateKey and IsValidPublicKey

func TestIsValidPrivateKey_Valid(t *testing.T) {
	key, _ := generateRSAKey(2048)
	keyPEM := encodeRSAPKCS1ToPEM(key)

	err := IsValidPrivateKey(keyPEM)
	if err != nil {
		t.Fatalf("IsValidPrivateKey should pass for valid key: %v", err)
	}
}

func TestIsValidPrivateKey_Invalid(t *testing.T) {
	err := IsValidPrivateKey([]byte("not a key"))
	if err == nil {
		t.Fatal("IsValidPrivateKey should fail for invalid key")
	}
}

func TestIsValidPublicKey_Valid(t *testing.T) {
	key, _ := generateRSAKey(2048)
	signer, _ := ssh.NewSignerFromKey(key)
	pubKey := ssh.MarshalAuthorizedKey(signer.PublicKey())

	err := IsValidPublicKey(string(pubKey))
	if err != nil {
		t.Fatalf("IsValidPublicKey should pass for valid key: %v", err)
	}
}

func TestIsValidPublicKey_Invalid(t *testing.T) {
	err := IsValidPublicKey("not a public key")
	if err == nil {
		t.Fatal("IsValidPublicKey should fail for invalid key")
	}
}
