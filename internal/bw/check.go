package bw

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/shichao402/pkv/internal/diag"
)

var bwVersionPattern = regexp.MustCompile(`\bv?(\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?)\b`)

// CheckBWInstalled checks if the Bitwarden CLI is available.
// If not, prints platform-specific install instructions and returns an error.
func CheckBWInstalled() error {
	return checkBWInstalled(exec.LookPath, exec.Command, os.Stdout)
}

func checkBWInstalled(lookPath func(string) (string, error), execCommand execCommandFunc, stdout io.Writer) error {
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	bwPath, err := lookPath("bw")
	if err != nil {
		printBWInstallHelp(stdout)
		return fmt.Errorf("bitwarden CLI (bw) not found in PATH")
	}

	version, err := detectBWVersion(execCommand)
	if err != nil {
		return fmt.Errorf("bitwarden CLI at %s failed version check: %w", bwPath, err)
	}

	diag.Printf("detected bw CLI at %s version=%q", bwPath, version)
	return nil
}

func detectBWVersion(execCommand execCommandFunc) (string, error) {
	if execCommand == nil {
		execCommand = exec.Command
	}

	cmd := execCommand("bw", "--version")
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if stderr != "" {
				return "", fmt.Errorf("`bw --version` failed: %s", stderr)
			}
		}
		return "", fmt.Errorf("`bw --version` failed: %w", err)
	}

	return parseBWVersion(string(out))
}

func parseBWVersion(output string) (string, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return "", fmt.Errorf("`bw --version` returned empty output")
	}

	match := bwVersionPattern.FindStringSubmatch(output)
	if len(match) != 2 {
		return "", fmt.Errorf("`bw --version` returned unexpected output %q; expected a version like 2026.2.0", output)
	}

	return match[1], nil
}

func printBWInstallHelp(stdout io.Writer) {
	if stdout == nil {
		stdout = io.Discard
	}

	fmt.Fprintln(stdout, "Bitwarden CLI (bw) is not installed.")
	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, "Install it using one of the following methods:")
	fmt.Fprintln(stdout, "")

	switch runtime.GOOS {
	case "darwin":
		fmt.Fprintln(stdout, "  # Homebrew (recommended)")
		fmt.Fprintln(stdout, "  brew install bitwarden-cli")
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "  # npm")
		fmt.Fprintln(stdout, "  npm install -g @bitwarden/cli")

	case "linux":
		fmt.Fprintln(stdout, "  # Snap")
		fmt.Fprintln(stdout, "  sudo snap install bw")
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "  # npm")
		fmt.Fprintln(stdout, "  npm install -g @bitwarden/cli")
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "  # Direct download")
		fmt.Fprintln(stdout, "  curl -fsSL https://vault.bitwarden.com/download/?app=cli&platform=linux -o bw.zip")
		fmt.Fprintln(stdout, "  unzip bw.zip && chmod +x bw && sudo mv bw /usr/local/bin/")

	case "windows":
		fmt.Fprintln(stdout, "  # Winget")
		fmt.Fprintln(stdout, "  winget install Bitwarden.CLI")
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "  # Chocolatey")
		fmt.Fprintln(stdout, "  choco install bitwarden-cli")
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "  # Scoop")
		fmt.Fprintln(stdout, "  scoop install bitwarden-cli")
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "  # npm")
		fmt.Fprintln(stdout, "  npm install -g @bitwarden/cli")

	default:
		fmt.Fprintln(stdout, "  # npm (cross-platform)")
		fmt.Fprintln(stdout, "  npm install -g @bitwarden/cli")
	}

	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, "For more info: https://bitwarden.com/help/cli/")
}
