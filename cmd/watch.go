package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shichao402/pkv/internal/bw"
	"github.com/shichao402/pkv/internal/guard"
	"github.com/shichao402/pkv/internal/state"
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Manage guard workspaces",
}

var watchListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered guard sync workspaces",
	Args:  cobra.NoArgs,
	RunE:  runWatchList,
}

var (
	watchAddFolder    string
	watchAddTargetDir string
)

var watchAddCmd = &cobra.Command{
	Use:   "add <root_path>",
	Short: "Register a workspace for note sync",
	Args:  cobra.ExactArgs(1),
	RunE:  runWatchAdd,
}

var watchRemoveCmd = &cobra.Command{
	Use:   "remove <root_path>",
	Short: "Remove a registered workspace",
	Args:  cobra.ExactArgs(1),
	RunE:  runWatchRemove,
}

var watchStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show guard status",
	Args:  cobra.NoArgs,
	RunE:  runWatchStatus,
}

func init() {
	watchAddCmd.Flags().StringVar(&watchAddFolder, "folder", "", "Bitwarden folder name (required)")
	watchAddCmd.Flags().StringVar(&watchAddTargetDir, "target-dir", "", "Directory to sync notes into; defaults to root_path")
	_ = watchAddCmd.MarkFlagRequired("folder")

	watchCmd.AddCommand(watchListCmd, watchAddCmd, watchRemoveCmd, watchStatusCmd)
	rootCmd.AddCommand(watchCmd)
}

func loadWatchGuard() (*guard.Guard, *state.State, error) {
	st, err := state.Load()
	if err != nil {
		return nil, nil, err
	}
	g := guard.New(st, bw.NewClient(), strings.TrimSpace(os.Getenv("BW_SESSION")))
	return g, st, nil
}

func runWatchList(cmd *cobra.Command, _ []string) error {
	_ = cmd
	_, st, err := loadWatchGuard()
	if err != nil {
		return err
	}
	workspaces := guard.ListRegisteredWorkspaces(st)
	type workspaceView struct {
		WorkspaceID  string `json:"workspace_id"`
		RootPath     string `json:"root_path"`
		Folder       string `json:"folder"`
		TargetDir    string `json:"target_dir"`
		RegisteredAt string `json:"registered_at"`
	}
	views := make([]workspaceView, 0, len(workspaces))
	for _, ws := range workspaces {
		views = append(views, workspaceView{
			WorkspaceID:  ws.RootPath,
			RootPath:     ws.RootPath,
			Folder:       ws.Folder,
			TargetDir:    ws.TargetDir,
			RegisteredAt: ws.RegisteredAt,
		})
	}
	text, err := json.MarshalIndent(map[string]any{"workspaces": views}, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(text))
	return nil
}

func runWatchAdd(cmd *cobra.Command, args []string) error {
	g, st, err := loadWatchGuard()
	if err != nil {
		return err
	}
	result, err := g.RegisterWorkspace(commandContext(cmd), args[0], watchAddFolder, watchAddTargetDir)
	if err != nil {
		return err
	}
	if err := st.Save(); err != nil {
		return err
	}
	text, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Printf("workspace registered: %s\n", result.Entry.RootPath)
		return nil
	}
	fmt.Println(string(text))
	return nil
}

func runWatchRemove(cmd *cobra.Command, args []string) error {
	_ = cmd
	_, st, err := loadWatchGuard()
	if err != nil {
		return err
	}
	if err := guard.UnregisterWorkspace(st, args[0]); err != nil {
		return err
	}
	if err := st.Save(); err != nil {
		return err
	}
	fmt.Printf("workspace unregistered: %s\n", args[0])
	return nil
}

func runWatchStatus(cmd *cobra.Command, _ []string) error {
	_ = cmd
	g, st, err := loadWatchGuard()
	if err != nil {
		return err
	}
	payload := map[string]any{
		"workspaces": len(guard.ListRegisteredWorkspaces(st)),
	}
	for k, v := range structToMap(g.Status()) {
		payload[k] = v
	}
	text, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(text))
	return nil
}

func structToMap(v any) map[string]any {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}
