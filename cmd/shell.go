package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
)

func runShell(_ *cobra.Command, _ []string) error {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Interactive mode. Type 'help' for commands, 'exit' to quit.")

	for {
		fmt.Print("pkv> ")
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
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
			rootCmd.SetArgs([]string{"--help"})
		case "list", "get", "add", "edit", "remove", "clean", "update", "completion", "version":
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

func resetShellCommandState() {
	addSSHPrivFlag = ""
	addSSHPubFlag = ""
	addNameFlag = ""
	addNoteFileFlag = ""
}

func translateShellArgs(args []string) ([]string, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("usage: <folder> <command>")
	}
	folder := args[0]
	verb := args[1]
	rest := args[2:]

	switch verb {
	case "list":
		return []string{"list", folder}, nil
	case "ssh", "env", "note":
		if len(rest) == 0 {
			return []string{"get", folder, verb}, nil
		}
		action := rest[0]
		tail := rest[1:]
		switch action {
		case "get":
			return []string{"get", folder, verb}, nil
		case "add":
			return append([]string{"add", folder, verb}, tail...), nil
		case "edit":
			return append([]string{"edit", folder, verb}, tail...), nil
		case "remove":
			return append([]string{"remove", folder, verb}, tail...), nil
		case "clean":
			return []string{"clean", folder, verb}, nil
		default:
			return nil, fmt.Errorf("unknown action: %s", action)
		}
	default:
		return nil, fmt.Errorf("unknown command: %s", verb)
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
