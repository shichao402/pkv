package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/shichao402/pkv/internal/version"
	"github.com/spf13/cobra"
)

const (
	githubRepo       = "shichao402/pkv"
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

	fmt.Printf("Downloading %s...\n", assetName)
	tmpFile, err := downloadAsset(downloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(tmpFile)

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current binary: %w", err)
	}

	// Replace current binary
	if err := replaceBinary(execPath, tmpFile); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}

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
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", err
	}
	tmpFile.Close()

	// Make executable
	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
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
		os.Rename(backupPath, targetPath)
		return fmt.Errorf("install new binary: %w", err)
	}

	// Remove backup
	os.Remove(backupPath)
	return nil
}
