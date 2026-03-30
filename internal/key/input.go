package key

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/shichao402/pkv/internal/pathutil"
)

// InputConfig holds flags from command-line for interactive prompting
type InputConfig struct {
	PrivatePath string // --priv flag
	PublicKey   string // --pub flag
	KeyName     string // --name flag
	Folder      string // folder name (from positional arg)
}

// InteractiveInput prompts user for missing inputs
func InteractiveInput(cfg *InputConfig) error {
	reader := bufio.NewReader(os.Stdin)

	// Get private key path
	for cfg.PrivatePath == "" {
		fmt.Print("Enter private key path (e.g., ~/.ssh/id_rsa): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read private key path failed: %w", err)
		}
		cfg.PrivatePath = strings.TrimSpace(input)

		// Expand ~ to home directory
		cfg.PrivatePath, err = pathutil.ExpandTilde(cfg.PrivatePath)
		if err != nil {
			return fmt.Errorf("resolve home directory: %w", err)
		}

		// Verify file exists
		if _, err := os.Stat(cfg.PrivatePath); err != nil {
			fmt.Printf("File not found: %s\n", cfg.PrivatePath)
			cfg.PrivatePath = ""
			continue
		}

		// Verify it's a valid private key
		keyBytes, _ := os.ReadFile(cfg.PrivatePath)
		if err := IsValidPrivateKey(keyBytes); err != nil {
			fmt.Printf("Invalid private key: %v\n", err)
			cfg.PrivatePath = ""
			continue
		}
	}

	// Get public key (optional - can auto-derive)
	if cfg.PublicKey == "" {
		fmt.Print("Enter public key (ssh-rsa AAAA...) or press Enter to auto-derive: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read public key failed: %w", err)
		}
		cfg.PublicKey = strings.TrimSpace(input)

		if cfg.PublicKey == "" {
			// Auto-derive from private key
			keyBytes, _ := os.ReadFile(cfg.PrivatePath)
			pubKey, err := GetPublicKeyFromPrivate(keyBytes)
			if err != nil {
				return fmt.Errorf("auto-derive public key failed: %w", err)
			}
			cfg.PublicKey = pubKey
			fmt.Printf("Auto-derived public key: %s\n", pubKey[:50]+"...")
		} else {
			// Validate provided public key
			keyBytes, _ := os.ReadFile(cfg.PrivatePath)
			_, signer, _ := parsePrivateKey(keyBytes)
			if err := ValidatePublicKey(cfg.PublicKey, signer); err != nil {
				fmt.Printf("Warning: public key may not match private key: %v\n", err)
				fmt.Print("Continue anyway? (yes/no): ")
				confirm, _ := reader.ReadString('\n')
				if strings.ToLower(strings.TrimSpace(confirm)) != "yes" {
					cfg.PublicKey = ""
					// Try auto-derive
					pubKey, _ := GetPublicKeyFromPrivate(keyBytes)
					cfg.PublicKey = pubKey
				}
			}
		}
	}

	// Get key name
	for cfg.KeyName == "" {
		fmt.Print("Enter key name (e.g., my-server-key): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read key name failed: %w", err)
		}
		cfg.KeyName = strings.TrimSpace(input)
		if cfg.KeyName == "" {
			fmt.Println("Key name cannot be empty")
		}
	}

	return nil
}

// ConfirmAndCreate shows a summary and asks for confirmation before creating
func ConfirmAndCreate(cfg *InputConfig, fingerprint string) (bool, error) {
	fmt.Println("\n=== Summary ===")
	fmt.Printf("Folder:        %s\n", cfg.Folder)
	fmt.Printf("Key Name:      %s\n", cfg.KeyName)
	fmt.Printf("Private Key:   %s\n", cfg.PrivatePath)
	fmt.Printf("Fingerprint:   %s\n", fingerprint)
	if len(cfg.PublicKey) > 50 {
		fmt.Printf("Public Key:    %s...\n", cfg.PublicKey[:50])
	} else {
		fmt.Printf("Public Key:    %s\n", cfg.PublicKey)
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\nCreate this SSH key in Bitwarden? (yes/no): ")
	confirm, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("read confirmation failed: %w", err)
	}

	return strings.ToLower(strings.TrimSpace(confirm)) == "yes", nil
}
