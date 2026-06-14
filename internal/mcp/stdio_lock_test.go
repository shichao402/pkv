package mcp

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireStdioLockExclusive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PKV_WORKSPACE_ROOT", filepath.Join(home, "ws"))

	first, err := acquireStdioLock()
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer releaseStdioLock(first)

	second, err := acquireStdioLock()
	if err == nil {
		releaseStdioLock(second)
		t.Fatal("expected second acquire to fail while first lock is held")
	}
}

func TestAcquireStdioLockReclaimsStaleHolder(t *testing.T) {
	home := t.TempDir()
	ws := filepath.Join(home, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	bin := buildPKVBinary(t, home)
	holder := exec.Command(bin, "mcp")
	holder.Env = append(os.Environ(),
		"HOME="+home,
		"PKV_WORKSPACE_ROOT="+ws,
	)
	if err := holder.Start(); err != nil {
		t.Fatalf("start holder: %v", err)
	}
	t.Cleanup(func() {
		if holder.Process != nil {
			_ = holder.Process.Kill()
		}
		_ = holder.Wait()
	})

	t.Setenv("HOME", home)
	t.Setenv("PKV_WORKSPACE_ROOT", ws)
	path, _, err := lockFilePath()
	if err != nil {
		t.Fatalf("lock path: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if readLockPID(path) == holder.Process.Pid {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if readLockPID(path) != holder.Process.Pid {
		t.Fatalf("holder pid=%d lock=%q", holder.Process.Pid, readLockContents(path))
	}

	lock, err := acquireStdioLock()
	if err != nil {
		t.Fatalf("reclaim acquire: %v", err)
	}
	releaseStdioLock(lock)

	waitDone := make(chan error, 1)
	go func() { waitDone <- holder.Wait() }()
	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected stale holder %d to exit after reclaim", holder.Process.Pid)
	}
}

func buildPKVBinary(t *testing.T, dir string) string {
	t.Helper()
	root := repoRoot(t)
	bin := filepath.Join(dir, "pkv")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build pkv: %v\n%s", err, out)
	}
	return bin
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func readLockContents(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return err.Error()
	}
	return string(data)
}
