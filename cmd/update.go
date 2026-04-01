package cmd

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shichao402/pkv/internal/version"
)

const (
	githubRepo        = "shichao402/pkv"
	githubReleasesURL = "https://github.com/" + githubRepo + "/releases/latest"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update pkv to the latest version",
	RunE:  runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(_ *cobra.Command, _ []string) error {
	fmt.Printf("Current version: %s\n", version.Version)
	fmt.Println("Checking for updates...")

	latestTag, err := fetchLatestTag()
	if err != nil {
		return fmt.Errorf("check update failed: %w", err)
	}

	latestVersion := strings.TrimPrefix(latestTag, "v")
	currentVersion := strings.TrimPrefix(version.Version, "v")

	if latestVersion == currentVersion && currentVersion != "dev" {
		fmt.Println("Already up to date.")
		return nil
	}

	fmt.Printf("New version available: %s\n", latestTag)

	assetName := buildAssetName()
	downloadURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", githubRepo, latestTag, assetName)

	// Download checksum file
	fmt.Println("Downloading checksums...")
	checksumURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/checksums.sha256", githubRepo, latestTag)
	expectedHash, err := fetchExpectedHash(checksumURL, assetName)
	if err != nil {
		return fmt.Errorf("checksum fetch failed: %w", err)
	}

	fmt.Printf("Downloading %s...\n", assetName)
	tmpFile, err := downloadAsset(downloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile) }()

	// Verify checksum
	fmt.Println("Verifying checksum...")
	if err := verifyChecksum(tmpFile, expectedHash); err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}
	fmt.Println("Checksum verified.")

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current binary: %w", err)
	}

	// Replace current binary
	if err := replaceBinary(execPath, tmpFile); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}

	// Remove macOS quarantine attribute
	removeQuarantineAttr(execPath)

	fmt.Printf("Updated to %s successfully.\n", latestTag)
	return nil
}

// fetchLatestTag gets the latest release tag via HTTP redirect (no API needed).
func fetchLatestTag() (string, error) {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(githubReleasesURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		return "", fmt.Errorf("expected redirect (302), got %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("no Location header in redirect response")
	}

	tag := path.Base(location)
	if tag == "" || tag == "." || tag == "/" {
		return "", fmt.Errorf("failed to extract tag from redirect URL: %s", location)
	}

	return tag, nil
}

// buildAssetName returns the expected asset filename for the current platform.
// Convention: pkv_{os}_{arch} (+ .exe on Windows)
func buildAssetName() string {
	name := fmt.Sprintf("pkv_%s_%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

func downloadAsset(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "pkv-update-*")
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		return "", err
	}
	_ = tmpFile.Close()

	// Make executable
	if err := os.Chmod(tmpFile.Name(), 0o755); err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

// fetchExpectedHash downloads the checksums file and extracts the expected hash for the given asset.
func fetchExpectedHash(checksumURL, assetName string) (string, error) {
	resp, err := http.Get(checksumURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download checksums returned HTTP %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		// Format: "<hash>  <filename>" (two spaces)
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == assetName {
			return parts[0], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read checksums: %w", err)
	}

	return "", fmt.Errorf("no checksum found for %s", assetName)
}

// verifyChecksum computes the SHA256 of the file and compares it to the expected hash.
func verifyChecksum(filePath, expectedHash string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	actualHash := hex.EncodeToString(h.Sum(nil))
	if actualHash != expectedHash {
		return fmt.Errorf("expected %s, got %s", expectedHash, actualHash)
	}
	return nil
}

// removeQuarantineAttr removes the macOS quarantine extended attribute.
func removeQuarantineAttr(path string) {
	if runtime.GOOS != "darwin" {
		return
	}
	if err := exec.Command("xattr", "-cr", path).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to remove quarantine attribute: %v\n", err)
	}
}

func replaceBinary(targetPath, newBinaryPath string) error {
	// Rename old binary as backup
	backupPath := targetPath + ".bak"
	if err := os.Rename(targetPath, backupPath); err != nil {
		return fmt.Errorf("backup old binary: %w", err)
	}

	// Move new binary into place
	if err := os.Rename(newBinaryPath, targetPath); err != nil {
		// Rollback: restore backup
		if rbErr := os.Rename(backupPath, targetPath); rbErr != nil {
			return fmt.Errorf("install new binary: %w (rollback also failed: %v; backup at %s)", err, rbErr, backupPath)
		}
		return fmt.Errorf("install new binary: %w (rolled back to previous version)", err)
	}

	// Remove backup (non-critical, warn on failure)
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Warning: failed to remove backup file %s: %v\n", backupPath, err)
	}
	return nil
}
