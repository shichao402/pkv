package key

import (
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestGenerateKeypair_Ed25519(t *testing.T) {
	priv, pub, fp, err := GenerateKeypair(AlgoEd25519, 0, "user@host")
	if err != nil {
		t.Fatalf("GenerateKeypair ed25519 failed: %v", err)
	}
	if priv == "" || pub == "" || fp == "" {
		t.Fatalf("empty result: priv=%q pub=%q fp=%q", priv, pub, fp)
	}

	// Private key must be parseable as an OpenSSH key.
	signer, err := ssh.ParsePrivateKey([]byte(priv))
	if err != nil {
		t.Fatalf("parse generated private key: %v", err)
	}

	// Public key string must start with ssh-ed25519 and end with the comment.
	if !strings.HasPrefix(pub, "ssh-ed25519 ") {
		t.Fatalf("public key prefix mismatch: %q", pub)
	}
	if !strings.HasSuffix(pub, " user@host") {
		t.Fatalf("public key comment missing: %q", pub)
	}

	// Fingerprint must match the one derived from the signer's public key.
	want := ssh.FingerprintSHA256(signer.PublicKey())
	if fp != want {
		t.Fatalf("fingerprint mismatch: got %q want %q", fp, want)
	}
}

func TestGenerateKeypair_RSA(t *testing.T) {
	priv, pub, fp, err := GenerateKeypair(AlgoRSA, 2048, "")
	if err != nil {
		t.Fatalf("GenerateKeypair rsa 2048 failed: %v", err)
	}
	if priv == "" || pub == "" || fp == "" {
		t.Fatal("empty result for rsa 2048")
	}
	if !strings.HasPrefix(pub, "ssh-rsa ") {
		t.Fatalf("rsa public key prefix mismatch: %q", pub)
	}
	if _, err := ssh.ParsePrivateKey([]byte(priv)); err != nil {
		t.Fatalf("parse generated rsa private key: %v", err)
	}

	if _, _, _, err := GenerateKeypair(AlgoRSA, 1024, ""); err == nil {
		t.Fatal("expected error for rsa bits < 2048, got nil")
	}
}

func TestGenerateKeypair_UnsupportedAlgo(t *testing.T) {
	if _, _, _, err := GenerateKeypair("ecdsa", 0, ""); err == nil {
		t.Fatal("expected error for unsupported algo, got nil")
	}
}

func TestGenerateKeypair_DefaultAlgoAndNoComment(t *testing.T) {
	// Empty algo => default ed25519.
	priv, pub, _, err := GenerateKeypair("", 0, "")
	if err != nil {
		t.Fatalf("default algo failed: %v", err)
	}
	if !strings.HasPrefix(pub, "ssh-ed25519 ") {
		t.Fatalf("default algo did not produce ed25519: %q", pub)
	}
	// No comment => public key is type + key material only (two whitespace-separated parts).
	if strings.Count(strings.TrimSpace(pub), " ") != 1 {
		t.Fatalf("expected no comment on public key, got %q", pub)
	}
	if _, err := ssh.ParsePrivateKey([]byte(priv)); err != nil {
		t.Fatalf("parse generated default private key: %v", err)
	}
}
