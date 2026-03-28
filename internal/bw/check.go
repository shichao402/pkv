package bw

import (
	"fmt"
	"os/exec"
	"runtime"
)

// CheckBWInstalled checks if the Bitwarden CLI is available.
// If not, prints platform-specific install instructions and returns an error.
func CheckBWInstalled() error {
	_, err := exec.LookPath("bw")
	if err == nil {
		return nil
	}

	fmt.Println("Bitwarden CLI (bw) is not installed.")
	fmt.Println("")
	fmt.Println("Install it using one of the following methods:")
	fmt.Println("")

	switch runtime.GOOS {
	case "darwin":
		fmt.Println("  # Homebrew (recommended)")
		fmt.Println("  brew install bitwarden-cli")
		fmt.Println("")
		fmt.Println("  # npm")
		fmt.Println("  npm install -g @bitwarden/cli")

	case "linux":
		fmt.Println("  # Snap")
		fmt.Println("  sudo snap install bw")
		fmt.Println("")
		fmt.Println("  # npm")
		fmt.Println("  npm install -g @bitwarden/cli")
		fmt.Println("")
		fmt.Println("  # Direct download")
		fmt.Println("  curl -fsSL https://vault.bitwarden.com/download/?app=cli&platform=linux -o bw.zip")
		fmt.Println("  unzip bw.zip && chmod +x bw && sudo mv bw /usr/local/bin/")

	case "windows":
		fmt.Println("  # Winget")
		fmt.Println("  winget install Bitwarden.CLI")
		fmt.Println("")
		fmt.Println("  # Chocolatey")
		fmt.Println("  choco install bitwarden-cli")
		fmt.Println("")
		fmt.Println("  # Scoop")
		fmt.Println("  scoop install bitwarden-cli")
		fmt.Println("")
		fmt.Println("  # npm")
		fmt.Println("  npm install -g @bitwarden/cli")

	default:
		fmt.Println("  # npm (cross-platform)")
		fmt.Println("  npm install -g @bitwarden/cli")
	}

	fmt.Println("")
	fmt.Println("For more info: https://bitwarden.com/help/cli/")

	return fmt.Errorf("bitwarden CLI (bw) not found in PATH")
}
