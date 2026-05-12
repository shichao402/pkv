package app

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/shichao402/pkv/internal/version"
)

const (
	githubRepo        = "shichao402/pkv"
	githubReleasesURL = "https://github.com/" + githubRepo + "/releases/latest"
)

type UpdateParams struct{}

type UpdateResult struct {
	CurrentVersion string
	LatestTag      string
	Updated        bool
}

func Update(ctx context.Context, _ UpdateParams, r Reporter) (UpdateResult, error) {
	r = reporterOrNoop(r)
	r.Infof("Current version: %s\n", version.Version)
	r.Info("Checking for updates...")

	latestTag, err := fetchLatestTag(ctx)
	if err != nil {
		return UpdateResult{}, fmt.Errorf("check update failed: %w", err)
	}

	latestVersion := strings.TrimPrefix(latestTag, "v")
	currentVersion := strings.TrimPrefix(version.Version, "v")
	result := UpdateResult{CurrentVersion: version.Version, LatestTag: latestTag}

	if latestVersion == currentVersion && currentVersion != "dev" {
		r.Info("Already up to date.")
		return result, nil
	}

	r.Infof("New version available: %s\n", latestTag)
	assetName := buildAssetName()
	downloadURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", githubRepo, latestTag, assetName)

	r.Info("Downloading checksums...")
	checksumURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/checksums.sha256", githubRepo, latestTag)
	expectedHash, err := fetchExpectedHash(ctx, checksumURL, assetName)
	if err != nil {
		return UpdateResult{}, fmt.Errorf("checksum fetch failed: %w", err)
	}

	execPath, err := os.Executable()
	if err != nil {
		return UpdateResult{}, fmt.Errorf("locate current binary: %w", err)
	}
	resolvedPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		resolvedPath = execPath
	}
	targetDir := filepath.Dir(resolvedPath)
	_ = os.Remove(resolvedPath + ".bak")

	r.Infof("Downloading %s...\n", assetName)
	tmpFile, err := downloadAsset(ctx, downloadURL, targetDir)
	if err != nil {
		return UpdateResult{}, fmt.Errorf("download failed: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile) }()

	r.Info("Verifying checksum...")
	if err := verifyChecksum(tmpFile, expectedHash); err != nil {
		return UpdateResult{}, fmt.Errorf("checksum verification failed: %w", err)
	}
	r.Info("Checksum verified.")

	if err := replaceBinary(resolvedPath, tmpFile); err != nil {
		return UpdateResult{}, fmt.Errorf("replace binary: %w", err)
	}
	removeQuarantineAttr(resolvedPath, r)
	r.Infof("Updated to %s successfully.\n", latestTag)
	result.Updated = true
	return result, nil
}

func fetchLatestTag(ctx context.Context) (string, error) {
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubReleasesURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
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

func buildAssetName() string {
	name := fmt.Sprintf("pkv_%s_%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

func downloadAsset(ctx context.Context, url, targetDir string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}
	tmpFile, err := os.CreateTemp(targetDir, ".pkv-update-*")
	if err != nil {
		tmpFile, err = os.CreateTemp("", "pkv-update-*")
		if err != nil {
			return "", err
		}
	}
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		return "", err
	}
	_ = tmpFile.Close()
	if err := os.Chmod(tmpFile.Name(), 0o755); err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", err
	}
	return tmpFile.Name(), nil
}

func fetchExpectedHash(ctx context.Context, checksumURL, assetName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download checksums returned HTTP %d", resp.StatusCode)
	}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) == 2 && parts[1] == assetName {
			return parts[0], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read checksums: %w", err)
	}
	return "", fmt.Errorf("no checksum found for %s", assetName)
}

func verifyChecksum(filePath, expectedHash string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
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

func removeQuarantineAttr(binPath string, r Reporter) {
	if runtime.GOOS != "darwin" {
		return
	}
	if err := exec.Command("xattr", "-cr", binPath).Run(); err != nil {
		reporterOrNoop(r).Warnf("Warning: failed to remove quarantine attribute: %v\n", err)
	}
}

func replaceBinary(targetPath, newBinaryPath string) error {
	backupPath := targetPath + ".bak"
	if err := os.Rename(targetPath, backupPath); err != nil {
		return fmt.Errorf("backup old binary: %w", err)
	}
	if err := os.Rename(newBinaryPath, targetPath); err != nil {
		if isCrossDeviceErr(err) {
			if cpErr := copyAndReplace(newBinaryPath, targetPath); cpErr == nil {
				_ = os.Remove(newBinaryPath)
				removeBackup(backupPath)
				return nil
			}
		}
		if rbErr := os.Rename(backupPath, targetPath); rbErr != nil {
			return fmt.Errorf("install new binary: %w (rollback also failed: %v; backup at %s)", err, rbErr, backupPath)
		}
		return fmt.Errorf("install new binary: %w (rolled back to previous version)", err)
	}
	removeBackup(backupPath)
	return nil
}

func removeBackup(backupPath string) {
	err := os.Remove(backupPath)
	if err == nil || os.IsNotExist(err) {
		return
	}
	if runtime.GOOS == "windows" {
		return
	}
}

func isCrossDeviceErr(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

func copyAndReplace(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	stagingPath := dst + ".new"
	out, err := os.OpenFile(stagingPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(stagingPath)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(stagingPath)
		return err
	}
	if err := os.Chmod(stagingPath, 0o755); err != nil {
		_ = os.Remove(stagingPath)
		return err
	}
	if err := os.Rename(stagingPath, dst); err != nil {
		_ = os.Remove(stagingPath)
		return err
	}
	return nil
}
