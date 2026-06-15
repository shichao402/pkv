//go:build windows

package mcp

import (
	"os"

	"golang.org/x/sys/windows"
)

func tryAcquireLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	handle := windows.Handle(f.Fd())
	var ol windows.Overlapped
	err = windows.LockFileEx(
		handle,
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, &ol,
	)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func releaseStdioLock(f *os.File) {
	if f == nil {
		return
	}
	handle := windows.Handle(f.Fd())
	var ol windows.Overlapped
	_ = windows.UnlockFileEx(handle, 0, 1, 0, &ol)
	_ = f.Close()
}

func findLockHolderPID(_ string) int {
	return 0
}

func processRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	_ = windows.CloseHandle(handle)
	return true
}

func signalStaleProcess(pid int) bool {
	return terminateProcess(pid)
}

func killStaleProcess(pid int) bool {
	return terminateProcess(pid)
}

func terminateProcess(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	return windows.TerminateProcess(handle, 1) == nil
}
