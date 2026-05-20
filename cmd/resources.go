package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shichao402/pkv/internal/app"
	"github.com/shichao402/pkv/internal/key"
	"github.com/shichao402/pkv/internal/pathutil"
	"github.com/shichao402/pkv/internal/securenote"
)

var (
	addSSHPrivFlag string
	addSSHPubFlag  string
	addNameFlag    string

	// --generate path flags (server-side keypair generation).
	addSSHGenerateFlag bool
	addSSHTypeFlag     string
	addSSHBitsFlag     int
	addSSHCommentFlag  string
	addSSHHostsFlag    []string

	// --authorize flag for `pkv get <folder> ssh`: append public keys to
	// ~/.ssh/authorized_keys after deploying.
	getSSHAuthorizeFlag bool

	addNoteFileFlag string

	// listResolvedFlag expands pkv.include when listing a single folder.
	// When false, list shows only the direct includes section and the
	// folder's own items; when true, env/note are merged across the
	// include chain and annotated with their source folder. SSH is
	// deliberately not expanded (MVP boundary from #114).
	listResolvedFlag bool
)

var listCmd = &cobra.Command{
	Use:     "list [folder]",
	Short:   "List folders or resources in a folder",
	Example: "  pkv list\n  pkv list prod",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		folder := ""
		if len(args) == 1 {
			folder = args[0]
		}
		_, err := app.List(commandContext(cmd), app.ListParams{Folder: folder, Resolved: listResolvedFlag}, cliReporter())
		return err
	},
}

var getCmd = &cobra.Command{
	Use:     "get <folder> <ssh|env|note|all>",
	Short:   "Get resources from a Bitwarden folder",
	Example: "  pkv get prod ssh\n  pkv get prod env\n  pkv get prod note\n  pkv get prod all",
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		folder, kind := args[0], args[1]
		reporter := cliReporter()
		if getSSHAuthorizeFlag && kind != "ssh" && kind != "all" {
			reporter.Warn("Warning: --authorize only applies to `ssh` (and `all`); ignoring")
		}
		_, err := app.Get(commandContext(cmd), app.GetParams{Folder: folder, Kind: kind, AuthorizeSSH: getSSHAuthorizeFlag}, reporter)
		return err
	},
}

var addCmd = &cobra.Command{
	Use:     "add <folder> <ssh|env|note>",
	Short:   "Create resources in a Bitwarden folder",
	Example: "  pkv add prod ssh --priv ~/.ssh/id_ed25519 --name github\n  pkv add prod env --file .env.prod\n  pkv add prod note --file ./xxx/aaa/bbb/test.json",
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		folder, kind := args[0], args[1]
		switch kind {
		case "ssh":
			return addSSHCmd.RunE(addSSHCmd, []string{folder})
		case "env":
			return addEnvCmd.RunE(addEnvCmd, []string{folder})
		case "note":
			return addNoteCmd.RunE(addNoteCmd, []string{folder})
		default:
			return fmt.Errorf("unknown resource type: %s (expected ssh, env, or note)", kind)
		}
	},
}

var folderCmd = &cobra.Command{
	Use:   "folder",
	Short: "Manage Bitwarden folders",
}

var folderAddCmd = &cobra.Command{
	Use:     "add <name>",
	Short:   "Create a Bitwarden folder",
	Example: "  pkv folder add prod",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := app.AddFolder(commandContext(cmd), app.AddFolderParams{Name: args[0]}, cliReporter())
		return err
	},
}

var addSSHCmd = &cobra.Command{
	Use:   "ssh <folder>",
	Short: "Add an SSH key to a folder",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		params, err := buildAddSSHKeyParams(args[0])
		if errors.Is(err, context.Canceled) {
			return nil
		}
		if err != nil {
			return err
		}
		_, err = app.AddSSHKey(commandContext(cmd), params, cliReporter())
		return err
	},
}

func buildAddSSHKeyParams(folder string) (app.AddSSHKeyParams, error) {
	cfg := &key.InputConfig{
		PrivatePath: addSSHPrivFlag,
		PublicKey:   addSSHPubFlag,
		KeyName:     addNameFlag,
		Folder:      folder,
	}

	var (
		opensshKey  string
		publicKey   string
		fingerprint string
		hosts       []string
		generated   bool
	)

	if addSSHGenerateFlag || cfg.PrivatePath == "" {
		generated = true
		if addSSHGenerateFlag && addSSHPrivFlag != "" {
			return app.AddSSHKeyParams{}, fmt.Errorf("--generate cannot be combined with --priv")
		}
		if addSSHPubFlag != "" {
			return app.AddSSHKeyParams{}, fmt.Errorf("--pub requires --priv")
		}
		if err := ensureSSHKeyName(cfg); err != nil {
			return app.AddSSHKeyParams{}, err
		}
		hosts = trimHosts(addSSHHostsFlag)
		comment := addSSHCommentFlag
		if comment == "" {
			comment = defaultKeyComment()
		}
		fmt.Printf("Adding SSH key to Bitwarden folder '%s'...\n", folder)
		fmt.Printf("Generating %s keypair in memory...\n", addSSHTypeFlag)
		var err error
		opensshKey, publicKey, fingerprint, err = key.GenerateKeypair(addSSHTypeFlag, addSSHBitsFlag, comment)
		if err != nil {
			return app.AddSSHKeyParams{}, fmt.Errorf("generate keypair: %w", err)
		}
	} else {
		if len(addSSHHostsFlag) > 0 {
			fmt.Fprintln(os.Stderr, "Warning: --host is only honored with --generate; ignoring (edit the item's notes in Bitwarden to set hosts)")
		}
		expandedPath, err := pathutil.ExpandTilde(cfg.PrivatePath)
		if err != nil {
			return app.AddSSHKeyParams{}, fmt.Errorf("resolve home directory: %w", err)
		}
		cfg.PrivatePath = expandedPath
		fmt.Printf("Adding SSH key to Bitwarden folder '%s'...\n", folder)
		if err := key.InteractiveInput(cfg); err != nil {
			return app.AddSSHKeyParams{}, fmt.Errorf("input failed: %w", err)
		}
		fmt.Printf("\nReading private key: %s\n", cfg.PrivatePath)
		privateKeyBytes, err := os.ReadFile(cfg.PrivatePath)
		if err != nil {
			return app.AddSSHKeyParams{}, fmt.Errorf("read private key failed: %w", err)
		}
		fmt.Println("Parsing and converting key...")
		opensshKey, publicKey, fingerprint, err = key.ParseAndConvertKey(privateKeyBytes)
		if err != nil {
			return app.AddSSHKeyParams{}, fmt.Errorf("parse key failed: %w", err)
		}
		confirm, err := key.ConfirmAndCreate(cfg, fingerprint)
		if err != nil {
			return app.AddSSHKeyParams{}, fmt.Errorf("confirmation failed: %w", err)
		}
		if !confirm {
			fmt.Println("Canceled.")
			return app.AddSSHKeyParams{}, context.Canceled
		}
	}

	return app.AddSSHKeyParams{
		Folder:      folder,
		KeyName:     cfg.KeyName,
		OpenSSHKey:  opensshKey,
		PublicKey:   publicKey,
		Fingerprint: fingerprint,
		Hosts:       hosts,
		Generated:   generated,
	}, nil
}

var addEnvCmd = &cobra.Command{
	Use:   "env <folder>",
	Short: "Create or replace the folder env note",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		content, err := readNoteContent(addNoteFileFlag, "Opening editor to write env content (KEY=VALUE format)...")
		if err != nil {
			return err
		}
		if strings.TrimSpace(content) == "" {
			fmt.Println("Empty content, canceled.")
			return nil
		}
		_, err = app.AddEnv(commandContext(cmd), app.AddParams{Folder: args[0], Content: content}, cliReporter())
		return err
	},
}

var addNoteCmd = &cobra.Command{
	Use:   "note <folder>",
	Short: "Create a config note in a folder",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		noteName, err := noteNameForAdd(addNameFlag, addNoteFileFlag)
		if err != nil {
			return err
		}
		content, err := readNoteContent(addNoteFileFlag, "Opening editor to write note content...")
		if err != nil {
			return err
		}
		if strings.TrimSpace(content) == "" {
			fmt.Println("Empty content, canceled.")
			return nil
		}
		_, err = app.AddNote(commandContext(cmd), app.AddParams{Folder: args[0], Name: noteName, Content: content}, cliReporter())
		return err
	},
}

var editCmd = &cobra.Command{
	Use:     "edit <folder> <env|note> [name-or-id]",
	Short:   "Edit resources in a Bitwarden folder",
	Example: "  pkv edit prod env\n  pkv edit prod note app.secrets.json",
	Args:    cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		folder, kind := args[0], args[1]
		switch kind {
		case "env":
			if len(args) != 2 {
				return fmt.Errorf("usage: pkv edit <folder> env")
			}
			_, err := app.Edit(commandContext(cmd), app.EditParams{Folder: folder, Kind: kind, EditNote: securenote.Edit}, cliReporter())
			return err
		case "note":
			if len(args) != 3 {
				return fmt.Errorf("usage: pkv edit <folder> note <name-or-id>")
			}
			_, err := app.Edit(commandContext(cmd), app.EditParams{Folder: folder, Kind: kind, NameOrID: args[2], EditNote: securenote.Edit}, cliReporter())
			return err
		default:
			return fmt.Errorf("unknown resource type: %s (expected env or note)", kind)
		}
	},
}

var removeCmd = &cobra.Command{
	Use:     "remove <folder> <ssh|env|note> [id...]",
	Short:   "Remove resources from Bitwarden",
	Example: "  pkv remove prod env\n  pkv remove prod ssh <item-id>\n  pkv remove prod note <item-id>",
	Args:    cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		folder, kind := args[0], args[1]
		switch kind {
		case "env":
			if len(args) != 2 {
				return fmt.Errorf("usage: pkv remove <folder> env")
			}
			_, err := app.Remove(commandContext(cmd), app.RemoveParams{Folder: folder, Kind: kind}, cliReporter())
			return err
		case "ssh", "note":
			if len(args) < 3 {
				return fmt.Errorf("usage: pkv remove <folder> %s <id> [id2...]", kind)
			}
			_, err := app.Remove(commandContext(cmd), app.RemoveParams{Folder: folder, Kind: kind, IDs: args[2:]}, cliReporter())
			return err
		default:
			return fmt.Errorf("unknown resource type: %s (expected ssh, env, or note)", kind)
		}
	},
}

var cleanCmd = &cobra.Command{
	Use:     "clean <folder> <ssh|env|note>",
	Short:   "Clean local materialized resources",
	Example: "  pkv clean prod ssh\n  pkv clean prod env\n  pkv clean prod note",
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := app.Clean(commandContext(cmd), app.CleanParams{Folder: args[0], Kind: args[1]}, cliReporter())
		return err
	},
}

func init() {
	rootCmd.AddCommand(listCmd, getCmd, addCmd, editCmd, removeCmd, cleanCmd, folderCmd)
	folderCmd.AddCommand(folderAddCmd)

	// --resolved applies to `pkv list <folder>`; the flag has no effect on
	// the bare `pkv list` (folders listing) and is silently ignored there.
	listCmd.Flags().BoolVar(&listResolvedFlag, "resolved", false, "Expand pkv.include and show merged env/notes with their source folder")

	addCmd.Flags().StringVar(&addSSHPrivFlag, "priv", "", "Private key file path (used with ssh)")
	addCmd.Flags().StringVar(&addSSHPubFlag, "pub", "", "Public key, ssh-rsa AAAA... format (used with ssh)")
	addCmd.Flags().StringVar(&addNameFlag, "name", "", "Item name in Bitwarden (used with ssh/note; note --file derives from relative path when omitted)")
	addCmd.Flags().StringVar(&addNoteFileFlag, "file", "", "File path to read content from (used with env/note)")

	// Server-side keypair generation flags (only relevant for `pkv add <folder> ssh`).
	addCmd.Flags().BoolVar(&addSSHGenerateFlag, "generate", false, "Generate a new SSH keypair in memory (alternative to --priv)")
	addCmd.Flags().StringVar(&addSSHTypeFlag, "type", "ed25519", "Key algorithm when --generate: ed25519|rsa")
	addCmd.Flags().IntVar(&addSSHBitsFlag, "bits", 4096, "RSA key size in bits (used with --generate --type rsa)")
	addCmd.Flags().StringVar(&addSSHCommentFlag, "comment", "", "Public key comment (default: <user>@<hostname> (pkv))")
	addCmd.Flags().StringSliceVar(&addSSHHostsFlag, "host", nil, "Target host(s) for ssh config (repeatable; defaults to local hostname when --generate)")

	// `pkv get <folder> ssh --authorize`: append every pulled public key to
	// the current host's ~/.ssh/authorized_keys (the ssh-copy-id role).
	getCmd.Flags().BoolVar(&getSSHAuthorizeFlag, "authorize", false, "After deploy, append each public key to ~/.ssh/authorized_keys on this host")
}

func noteNameForAdd(nameFlag, fileFlag string) (string, error) {
	if nameFlag != "" || fileFlag == "" {
		return nameFlag, nil
	}
	name, err := pathutil.RelativeFileNoteName(fileFlag)
	if err != nil {
		return "", fmt.Errorf("derive note name from file: %w", err)
	}
	return name, nil
}

func readNoteContent(fileFlag, openEditorMessage string) (string, error) {
	if fileFlag != "" {
		filePath, err := pathutil.ExpandTilde(fileFlag)
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("read file: %w", err)
		}
		return string(data), nil
	}

	fmt.Println(openEditorMessage)
	edited, err := securenote.OpenEditor("")
	if err != nil {
		return "", fmt.Errorf("editor: %w", err)
	}
	return edited, nil
}

func ensureSSHKeyName(cfg *key.InputConfig) error {
	reader := bufio.NewReader(os.Stdin)
	for strings.TrimSpace(cfg.KeyName) == "" {
		fmt.Print("Enter key name (e.g., my-server-key): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read key name failed: %w", err)
		}
		cfg.KeyName = strings.TrimSpace(input)
		if cfg.KeyName == "" {
			fmt.Println("Key name cannot be empty")
		}
	}
	cfg.KeyName = strings.TrimSpace(cfg.KeyName)
	return nil
}

func defaultKeyComment() string {
	u := os.Getenv("USER")
	if u == "" {
		u = "user"
	}
	h, _ := os.Hostname()
	if h == "" {
		h = "host"
	}
	if ip := localIPv4(); ip != "" {
		return fmt.Sprintf("%s@%s [%s] (pkv)", u, h, ip)
	}
	return fmt.Sprintf("%s@%s (pkv)", u, h)
}

func localIPv4() string {
	return selectIPv4(localIPv4Candidates())
}

type ipv4Candidate struct {
	iface string
	ip    net.IP
}

func localIPv4Candidates() []ipv4Candidate {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []ipv4Candidate
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		if isVirtualIface(ifi.Name) {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			ip4 := ip.To4()
			if ip4 == nil {
				continue
			}
			if ip4.IsLoopback() || ip4.IsLinkLocalUnicast() || ip4.IsUnspecified() {
				continue
			}
			out = append(out, ipv4Candidate{iface: ifi.Name, ip: ip4})
		}
	}
	return out
}

var virtualIfacePrefixes = []string{
	"docker", "br-", "veth", "virbr", "vnet", "vmnet", "vboxnet", "tailscale",
	"tun", "tap", "utun", "wg", "zt", "cni", "flannel", "cali", "weave", "ppp",
	"awdl", "llw", "gif", "stf",
}

var virtualIfaceExactNames = map[string]struct{}{
	"ap1": {}, "bridge0": {}, "bridge1": {}, "anpi0": {}, "anpi1": {},
}

func isVirtualIface(name string) bool {
	lower := strings.ToLower(name)
	for _, p := range virtualIfacePrefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	_, ok := virtualIfaceExactNames[lower]
	return ok
}

func selectIPv4(candidates []ipv4Candidate) string {
	if len(candidates) == 0 {
		return ""
	}
	sorted := make([]ipv4Candidate, len(candidates))
	copy(sorted, candidates)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].iface != sorted[j].iface {
			return sorted[i].iface < sorted[j].iface
		}
		return string(sorted[i].ip) < string(sorted[j].ip)
	})
	var firstPrivate string
	for _, c := range sorted {
		if !isPrivateIPv4(c.ip) {
			return c.ip.String()
		}
		if firstPrivate == "" {
			firstPrivate = c.ip.String()
		}
	}
	return firstPrivate
}

func isPrivateIPv4(ip net.IP) bool {
	if len(ip) != 4 {
		return false
	}
	switch {
	case ip[0] == 10:
		return true
	case ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31:
		return true
	case ip[0] == 192 && ip[1] == 168:
		return true
	case ip[0] == 100 && ip[1] >= 64 && ip[1] <= 127:
		return true
	}
	return false
}

func trimHosts(flagHosts []string) []string {
	out := make([]string, 0, len(flagHosts))
	for _, h := range flagHosts {
		if h = strings.TrimSpace(h); h != "" {
			out = append(out, h)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
