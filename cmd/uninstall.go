package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shichao402/pkv/internal/uninstall"
)

var uninstallYes bool

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall pkv and remove local PKV data",
	Long: `Uninstall pkv from this machine.

Delegates cleanup to an embedded Python 3 script (stdlib only) so Windows,
macOS, and Linux share one implementation. The script removes:

  - the pkv binary (with Windows self-delete handling)
  - ~/.pkv (state, session, env artifacts, backups, locks)
  - ~/.ssh/pkv_* keys and PKV-managed SSH config / known_hosts blocks
  - tracked local note files recorded in state
  - Windows user PATH entry for the install directory
  - legacy shell rc sourcing lines and optional MCP server entries

Requires a working python3 / python interpreter on PATH.
Does not delete anything from Bitwarden.`,
	RunE: runUninstall,
}

func init() {
	uninstallCmd.Flags().BoolVarP(&uninstallYes, "yes", "y", false, "Skip confirmation prompt")
	rootCmd.AddCommand(uninstallCmd)
}

func runUninstall(cmd *cobra.Command, _ []string) error {
	if !uninstallYes {
		fmt.Fprint(cmd.OutOrStdout(), "This will permanently remove local PKV data and the binary. Continue? [y/N] ")
		var reply string
		if _, err := fmt.Fscanln(cmd.InOrStdin(), &reply); err != nil {
			reply = ""
		}
		reply = strings.TrimSpace(strings.ToLower(reply))
		if reply != "y" && reply != "yes" {
			fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
			return nil
		}
	}

	python, pythonArgs, err := findPython()
	if err != nil {
		return err
	}

	scriptPath, err := writeTempUninstallScript()
	if err != nil {
		return err
	}

	exePath, err := os.Executable()
	if err != nil {
		_ = os.Remove(scriptPath)
		return fmt.Errorf("locate current binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}

	args := append(append([]string{}, pythonArgs...), scriptPath,
		"--yes",
		"--exe", exePath,
		"--pid", fmt.Sprintf("%d", os.Getpid()),
		"--script", scriptPath,
	)

	proc := exec.Command(python, args...)
	proc.Stdout = cmd.OutOrStdout()
	proc.Stderr = cmd.ErrOrStderr()
	proc.Stdin = nil
	detachUninstallProcess(proc)

	fmt.Fprintln(cmd.OutOrStdout(), "Starting uninstall helper...")
	if err := proc.Start(); err != nil {
		_ = os.Remove(scriptPath)
		return fmt.Errorf("start uninstall helper: %w", err)
	}

	// Exit immediately so Windows can unlock the running .exe for deletion.
	// The helper waits for this PID, then finishes cleanup.
	os.Exit(0)
	return nil
}

func writeTempUninstallScript() (string, error) {
	dir := os.TempDir()
	f, err := os.CreateTemp(dir, "pkv-uninstall-*.py")
	if err != nil {
		return "", fmt.Errorf("create temp uninstall script: %w", err)
	}
	path := f.Name()
	if _, err := f.WriteString(uninstall.Script); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write temp uninstall script: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func findPython() (string, []string, error) {
	candidates := []struct {
		bin  string
		args []string
	}{
		{"python3", nil},
		{"python", nil},
	}
	if runtime.GOOS == "windows" {
		candidates = append(candidates, struct {
			bin  string
			args []string
		}{"py", []string{"-3"}})
	}

	var tried []string
	for _, c := range candidates {
		path, err := exec.LookPath(c.bin)
		if err != nil {
			tried = append(tried, c.bin)
			continue
		}
		args := append([]string{}, c.args...)
		checkArgs := append(append([]string{}, args...), "--version")
		out, err := exec.Command(path, checkArgs...).CombinedOutput()
		versionLine := strings.TrimSpace(string(out))
		if err != nil || !strings.Contains(strings.ToLower(versionLine), "python") {
			tried = append(tried, c.bin)
			continue
		}
		// Avoid the Windows Store python stub that prints an install hint and exits 0/9009 oddly.
		if runtime.GOOS == "windows" && strings.Contains(strings.ToLower(versionLine), "microsoft store") {
			tried = append(tried, c.bin+" (store stub)")
			continue
		}
		return path, args, nil
	}

	return "", nil, fmt.Errorf("python 3 is required for `pkv uninstall` (tried: %s); install Python 3 or run the standalone uninstall.py from the repository", strings.Join(tried, ", "))
}
