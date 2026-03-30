package cmd

import (
	"fmt"
	"os"

	"github.com/shichao402/pkv/internal/key"
	"github.com/shichao402/pkv/internal/state"
	"github.com/spf13/cobra"
)

var keyCmd = &cobra.Command{
	Use:   "key",
	Short: "Manage SSH keys stored in Bitwarden",
	Long:  `Manage SSH keys that are stored in your Bitwarden vault.`,
}

var keyAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add an SSH private key to Bitwarden vault",
	Long: `Add an SSH private key to Bitwarden vault as a native SSH Key item.

Supports PEM (PKCS1/PKCS8/EC) and OpenSSH formats, with automatic
conversion to OpenSSH format. Public key can be auto-derived from private key.

Examples:
  pkv key add --priv ~/.ssh/id_rsa --pub "ssh-rsa AAAA..." --name "my-key"
  pkv key add  # Interactive mode`,
	Args: cobra.NoArgs,
	RunE: runKeyAdd,
}

var (
	keyAddPrivFlag string
	keyAddPubFlag  string
	keyAddNameFlag string
)

func init() {
	rootCmd.AddCommand(keyCmd)
	keyCmd.AddCommand(keyAddCmd)
	keyAddCmd.Flags().StringVar(&keyAddPrivFlag, "priv", "", "Private key file path")
	keyAddCmd.Flags().StringVar(&keyAddPubFlag, "pub", "", "Public key (ssh-rsa AAAA... format)")
	keyAddCmd.Flags().StringVar(&keyAddNameFlag, "name", "", "Key name in Bitwarden")
}

func runKeyAdd(_ *cobra.Command, _ []string) error {
	// Prepare input configuration
	cfg := &key.InputConfig{
		PrivatePath: keyAddPrivFlag,
		PublicKey:   keyAddPubFlag,
		KeyName:     keyAddNameFlag,
	}

	// Get missing inputs interactively
	fmt.Println("Adding SSH key to Bitwarden...")
	if err := key.InteractiveInput(cfg); err != nil {
		return fmt.Errorf("input failed: %w", err)
	}

	// Read and validate private key
	fmt.Printf("\nReading private key: %s\n", cfg.PrivatePath)
	privateKeyBytes, err := os.ReadFile(cfg.PrivatePath)
	if err != nil {
		return fmt.Errorf("read private key failed: %w", err)
	}

	// Parse and convert key
	fmt.Println("Parsing and converting key...")
	opensshKey, publicKey, fingerprint, err := key.ParseAndConvertKey(privateKeyBytes)
	if err != nil {
		return fmt.Errorf("parse key failed: %w", err)
	}

	// Ensure Bitwarden is unlocked
	fmt.Println("Authenticating with Bitwarden...")
	session, err := key.EnsureBWUnlocked()
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Confirm with user
	confirm, err := key.ConfirmAndCreate(cfg, fingerprint)
	if err != nil {
		return fmt.Errorf("confirmation failed: %w", err)
	}
	if !confirm {
		fmt.Println("Cancelled")
		return nil
	}

	// Create SSH key item in Bitwarden
	fmt.Println("Creating SSH key in Bitwarden...")
	itemID, err := key.CreateBWSSHKey(session, cfg.KeyName, opensshKey, publicKey, fingerprint)
	if err != nil {
		return fmt.Errorf("create SSH key in Bitwarden failed: %w", err)
	}

	// Record in state
	fmt.Println("Recording in state...")
	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state failed: %w", err)
	}

	st.AddStoredSSHKey(itemID, cfg.KeyName, fingerprint)
	if err := st.Save(); err != nil {
		return fmt.Errorf("save state failed: %w", err)
	}

	fmt.Printf("\n✓ SSH key '%s' successfully added to Bitwarden\n", cfg.KeyName)
	fmt.Printf("  Item ID: %s\n", itemID)
	fmt.Printf("  Fingerprint: %s\n", fingerprint)

	return nil
}
