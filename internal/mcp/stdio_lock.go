package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// acquireStdioLock ensures at most one pkv mcp serves a given PKV_WORKSPACE_ROOT.
// When Cursor reloads MCP, a previous pkv mcp child may still hold the lock; reclaim it once.
func acquireStdioLock() (*os.File, error) {
	path, ws, err := lockFilePath()
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 2; attempt++ {
		f, err := tryAcquireLock(path)
		if err == nil {
			if err := writeLockOwner(f); err != nil {
				releaseStdioLock(f)
				return nil, err
			}
			return f, nil
		}
		if attempt == 0 && terminateStaleLockHolder(path) {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if ws == "" {
			return nil, fmt.Errorf("another pkv mcp is already running (set PKV_WORKSPACE_ROOT or stop stale pkv mcp processes)")
		}
		return nil, fmt.Errorf("another pkv mcp is already running for workspace %q", ws)
	}
	return nil, fmt.Errorf("failed to acquire mcp lock")
}

func lockFilePath() (path, workspace string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	dir := filepath.Join(home, ".pkv")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	ws := strings.TrimSpace(os.Getenv("PKV_WORKSPACE_ROOT"))
	suffix := "default"
	if ws != "" {
		suffix = strings.ReplaceAll(ws, string(os.PathSeparator), "_")
	}
	return filepath.Join(dir, "mcp_"+suffix+".lock"), ws, nil
}

func writeLockOwner(f *os.File) error {
	if err := f.Truncate(0); err != nil {
		return err
	}
	_, err := fmt.Fprintf(f, "%d\n", os.Getpid())
	return err
}

func terminateStaleLockHolder(path string) bool {
	pid := readLockPID(path)
	if pid <= 0 || pid == os.Getpid() {
		pid = findLockHolderPID(path)
	}
	if pid <= 0 || pid == os.Getpid() {
		return false
	}
	if !signalStaleProcess(pid) {
		return false
	}
	for range 5 {
		if !processRunning(pid) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !killStaleProcess(pid) {
		return false
	}
	for range 5 {
		if !processRunning(pid) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func readLockPID(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	line := strings.TrimSpace(string(data))
	if line == "" {
		return 0
	}
	pid, err := strconv.Atoi(strings.Fields(line)[0])
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}
