package ssh

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shichao402/pkv/internal/bw/types"
	"github.com/shichao402/pkv/internal/state"
)

type Deployer struct {
	state  *state.State
	sshDir string
}

func NewDeployer(st *state.State) *Deployer {
	home, _ := os.UserHomeDir()
	return &Deployer{
		state:  st,
		sshDir: filepath.Join(home, ".ssh"),
	}
}

// Deploy writes an SSH key to ~/.ssh/ and updates ~/.ssh/config and ~/.ssh/known_hosts.
func (d *Deployer) Deploy(item types.Item) error {
	if item.SSHKey == nil {
		return fmt.Errorf("item '%s' has no SSH key data", item.Name)
	}

	keyName := sanitizeName(item.Name)
	keyFile := filepath.Join(d.sshDir, "pkv_"+keyName)
	pubFile := keyFile + ".pub"

	// Ensure ~/.ssh/ exists
	if err := os.MkdirAll(d.sshDir, 0o700); err != nil {
		return fmt.Errorf("create .ssh dir: %w", err)
	}

	// Write private key
	privateKey := item.SSHKey.PrivateKey
	if !strings.HasSuffix(privateKey, "\n") {
		privateKey += "\n"
	}
	if err := os.WriteFile(keyFile, []byte(privateKey), 0o600); err != nil {
		return fmt.Errorf("write private key: %w", err)
	}

	// Write public key
	if item.SSHKey.PublicKey != "" {
		pubKey := item.SSHKey.PublicKey
		if !strings.HasSuffix(pubKey, "\n") {
			pubKey += "\n"
		}
		if err := os.WriteFile(pubFile, []byte(pubKey), 0o644); err != nil {
			return fmt.Errorf("write public key: %w", err)
		}
	}

	// Hosts come from the item's Notes field (one host per line)
	hosts := item.GetHosts()

	// Update SSH config
	if len(hosts) > 0 {
		configPath := filepath.Join(d.sshDir, "config")
		if err := AddHostEntries(configPath, keyName, keyFile, hosts); err != nil {
			return fmt.Errorf("update ssh config: %w", err)
		}
	}

	// Record in state
	d.state.AddSSHKey(state.SSHKeyEntry{
		ItemID:  item.ID,
		KeyName: keyName,
		KeyFile: keyFile,
		PubFile: pubFile,
		Hosts:   hosts,
	})

	return nil
}

// DeployKnownHosts runs ssh-keyscan for all hosts across all deployed keys.
// Call this once after all keys are deployed.
func (d *Deployer) DeployKnownHosts(allHosts []string) error {
	return ScanAndAddKnownHosts(d.sshDir, allHosts)
}

// Remove removes a deployed SSH key and its config entries.
func (d *Deployer) Remove(entry state.SSHKeyEntry) error {
	var errs []string

	if err := os.Remove(entry.KeyFile); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Sprintf("remove private key %s: %v", entry.KeyFile, err))
	}
	if err := os.Remove(entry.PubFile); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Sprintf("remove public key %s: %v", entry.PubFile, err))
	}

	if len(entry.Hosts) > 0 {
		configPath := filepath.Join(d.sshDir, "config")
		if err := RemoveHostEntries(configPath, entry.KeyName); err != nil {
			errs = append(errs, fmt.Sprintf("remove ssh config entries: %v", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("cleanup errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// RemoveAllKnownHosts removes the PKV managed block from known_hosts.
// Call this once after all keys are removed.
func (d *Deployer) RemoveAllKnownHosts() error {
	return RemoveKnownHosts(d.sshDir)
}

func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('_')
		}
	}
	return b.String()
}
