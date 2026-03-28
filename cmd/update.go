package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/shichao402/pkv/internal/version"
	"github.com/spf13/cobra"
)

const (
	githubRepo   = "shichao402/pkv"
	githubAPIURL = "https://api.github.com/repos/" + githubRepo + "/releases/latest"
)

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

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

	release, err := fetchLatestRelease()
	if err != nil {
		return fmt.Errorf("check update failed: %w", err)
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimPrefix(version.Version, "v")

	if latestVersion == currentVersion && currentVersion != "dev" {
		fmt.Println("Already up to date.")
		return nil
	}

	fmt.Printf("New version available: %s\n", release.TagName)

	assetName := buildAssetName()
	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("no release asset found for %s/%s (looking for %s)", runtime.GOOS, runtime.GOARCH, assetName)
	}

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

	fmt.Printf("Updated to %s successfully.\n", release.TagName)
	return nil
}

func fetchLatestRelease() (*ghRelease, error) {
	resp, err := http.Get(githubAPIURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
	}

	var release ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
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
