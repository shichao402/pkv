package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/chzyer/readline"
	"github.com/spf13/cobra"

	"github.com/shichao402/pkv/internal/diag"
)

func runShell(_ *cobra.Command, _ []string) error {
	rl, err := newShellReadline()
	if err != nil {
		return fmt.Errorf("initialize interactive shell: %w", err)
	}
	defer rl.Close()
	rl.CaptureExitSignal()

	fmt.Println("Interactive mode. Type 'help' for commands, 'exit' to quit.")
	fmt.Println("Examples: 'get dev env' or 'dev env'.")

	for {
		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			if strings.TrimSpace(line) == "" {
				fmt.Println()
				continue
			}
		} else if err != nil && err != io.EOF {
			return err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			if err == io.EOF {
				fmt.Println()
				return nil
			}
			continue
		}

		args, parseErr := parseShellArgs(line)
		if parseErr != nil {
			fmt.Fprintln(os.Stderr, parseErr)
			if err == io.EOF {
				return nil
			}
			continue
		}

		if len(args) > 0 && args[0] == "pkv" {
			args = args[1:]
		}
		diag.Printf("shell parsed input %q -> %v", line, args)
		if len(args) == 0 {
			if err == io.EOF {
				return nil
			}
			continue
		}

		resetShellCommandState()
		switch args[0] {
		case "exit", "quit":
			return nil
		case "help":
			diag.Printf("shell dispatching built-in help")
			rootCmd.SetArgs([]string{"--help"})
		case "list", "get", "add", "edit", "remove", "clean", "update", "completion", "version":
			diag.Printf("shell executing direct command %v", args)
			rootCmd.SetArgs(args)
		default:
			translated, translateErr := translateShellArgs(args)
			if translateErr != nil {
				fmt.Fprintln(os.Stderr, translateErr)
				if err == io.EOF {
					return nil
				}
				continue
			}
			diag.Printf("shell translated command %v -> %v", args, translated)
			rootCmd.SetArgs(translated)
		}

		if _, execErr := rootCmd.ExecuteC(); execErr != nil {
			fmt.Fprintln(os.Stderr, execErr)
		}

		if err == io.EOF {
			return nil
		}
	}
}

func newShellReadline() (*readline.Instance, error) {
	historyPath, err := shellHistoryPath()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(historyPath), 0o700); err != nil {
		return nil, fmt.Errorf("prepare shell history directory: %w", err)
	}

	return readline.NewEx(&readline.Config{
		Prompt:            "pkv> ",
		HistoryFile:       historyPath,
		HistorySearchFold: true,
		InterruptPrompt:   "",
		EOFPrompt:         "",
	})
}

func shellHistoryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return shellHistoryPathFromHome(home), nil
}

func shellHistoryPathFromHome(home string) string {
	return filepath.Join(home, ".pkv", "shell_history")
}

func resetShellCommandState() {
	addSSHPrivFlag = ""
	addSSHPubFlag = ""
	addNameFlag = ""
	addNoteFileFlag = ""
}

func translateShellArgs(args []string) ([]string, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("usage: <folder> <list|get|add|edit|remove|clean|ssh|env|note> ...")
	}
	folder := args[0]
	second := args[1]
	rest := args[2:]

	switch {
	case second == "list":
		if len(rest) > 0 {
			return nil, fmt.Errorf("usage: <folder> list")
		}
		return []string{"list", folder}, nil
	case isShellResourceKind(second):
		return translateResourceFirstArgs(folder, second, rest)
	default:
		return nil, fmt.Errorf("unknown command: %s", second)
	}
}

func translateResourceFirstArgs(folder, kind string, rest []string) ([]string, error) {
	if len(rest) == 0 {
		return []string{"get", folder, kind}, nil
	}
	action := rest[0]
	if !isShellResourceSubcommand(action) {
		return nil, fmt.Errorf("unknown action: %s", action)
	}
	return append([]string{action, folder, kind}, rest[1:]...), nil
}

func isShellResourceKind(s string) bool {
	switch s {
	case "ssh", "env", "note":
		return true
	default:
		return false
	}
}

func isShellResourceSubcommand(s string) bool {
	switch s {
	case "add", "edit", "remove", "clean":
		return true
	default:
		return false
	}
}

func parseShellArgs(line string) ([]string, error) {
	var args []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	flush := func() {
		if current.Len() == 0 {
			return
		}
		args = append(args, current.String())
		current.Reset()
	}

	for _, r := range line {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\' && !inSingle:
			escaped = true
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case unicode.IsSpace(r) && !inSingle && !inDouble:
			flush()
		default:
			current.WriteRune(r)
		}
	}

	if escaped {
		return nil, fmt.Errorf("unterminated escape")
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unterminated quote")
	}
	flush()
	return args, nil
}
