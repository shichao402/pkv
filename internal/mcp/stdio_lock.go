package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// acquireStdioLock ensures at most one pkv mcp serves a given PKV_WORKSPACE_ROOT.
func acquireStdioLock() (*os.File, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".pkv")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	ws := strings.TrimSpace(os.Getenv("PKV_WORKSPACE_ROOT"))
	suffix := "default"
	if ws != "" {
		suffix = strings.ReplaceAll(ws, string(os.PathSeparator), "_")
	}
	path := filepath.Join(dir, "mcp_"+suffix+".lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if ws == "" {
			return nil, fmt.Errorf("another pkv mcp is already running (set PKV_WORKSPACE_ROOT or stop stale pkv mcp processes)")
		}
		return nil, fmt.Errorf("another pkv mcp is already running for workspace %q", ws)
	}
	return f, nil
}

func releaseStdioLock(f *os.File) {
	if f == nil {
		return
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	_ = f.Close()
}
