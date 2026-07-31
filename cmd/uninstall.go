package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

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

On Windows the helper runs detached and writes to a log file, because the
running pkv.exe must exit before it can be deleted. Elsewhere the helper runs
in the foreground and streams its progress.

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
		if r := strings.TrimSpace(strings.ToLower(reply)); r != "y" && r != "yes" {
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
		"--script", scriptPath,
	)

	if runtime.GOOS == "windows" {
		return startDetachedUninstall(cmd, python, args, scriptPath)
	}
	return runForegroundUninstall(cmd, python, args, scriptPath)
}

// runForegroundUninstall streams helper output. Unix can unlink the running
// binary, so there is no need to exit first.
func runForegroundUninstall(cmd *cobra.Command, python string, args []string, scriptPath string) error {
	args = append(args, "--ignore-pid", fmt.Sprintf("%d", os.Getpid()))

	proc := exec.Command(python, args...)
	proc.Stdout = cmd.OutOrStdout()
	proc.Stderr = cmd.ErrOrStderr()

	if err := proc.Run(); err != nil {
		return fmt.Errorf("uninstall helper failed: %w", err)
	}
	return nil
}

// startDetachedUninstall spawns the helper without a console and exits, so the
// helper can delete this .exe once the process is gone. A detached process
// cannot use the inherited console handles, so its output goes to a log file.
func startDetachedUninstall(cmd *cobra.Command, python string, args []string, scriptPath string) error {
	logPath := filepath.Join(os.TempDir(), fmt.Sprintf("pkv-uninstall-%d.log", time.Now().Unix()))
	logFile, err := os.Create(logPath)
	if err != nil {
		_ = os.Remove(scriptPath)
		return fmt.Errorf("create uninstall log: %w", err)
	}
	defer func() { _ = logFile.Close() }()

	args = append(args, "--pid", fmt.Sprintf("%d", os.Getpid()), "--log", logPath)

	proc := exec.Command(python, args...)
	proc.Stdout = logFile
	proc.Stderr = logFile
	detachUninstallProcess(proc)

	if err := proc.Start(); err != nil {
		_ = os.Remove(scriptPath)
		return fmt.Errorf("start uninstall helper: %w", err)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Uninstall helper started in the background.")
	fmt.Fprintf(out, "It waits for this process (pid %d) to exit, then removes pkv.exe.\n", os.Getpid())
	fmt.Fprintf(out, "Progress log: %s\n", logPath)

	// Exit now so Windows unlocks the running binary for deletion.
	os.Exit(0)
	return nil
}

func writeTempUninstallScript() (string, error) {
	f, err := os.CreateTemp(os.TempDir(), "pkv-uninstall-*.py")
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
	type candidate struct {
		bin  string
		args []string
	}

	candidates := []candidate{{"python3", nil}, {"python", nil}}
	if runtime.GOOS == "windows" {
		candidates = append(candidates, candidate{"py", []string{"-3"}})
	}

	var tried []string
	for _, c := range candidates {
		path, err := exec.LookPath(c.bin)
		if err != nil {
			tried = append(tried, c.bin)
			continue
		}
		args := append([]string{}, c.args...)
		out, err := exec.Command(path, append(append([]string{}, args...), "--version")...).CombinedOutput()
		versionLine := strings.ToLower(strings.TrimSpace(string(out)))
		if err != nil || !strings.Contains(versionLine, "python") {
			tried = append(tried, c.bin)
			continue
		}
		// The Windows Store stub answers `--version` without being a real interpreter.
		if strings.Contains(versionLine, "microsoft store") {
			tried = append(tried, c.bin+" (store stub)")
			continue
		}
		return path, args, nil
	}

	return "", nil, fmt.Errorf("python 3 is required for `pkv uninstall` (tried: %s); install Python 3 or run the standalone uninstall.py from the repository", strings.Join(tried, ", "))
}
