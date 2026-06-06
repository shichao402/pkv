package bw

import (
	"io"
	"os/exec"
	"testing"
)

func TestCheckBWInstalledRunsVersionOncePerProcess(t *testing.T) {
	resetBWInstalledCacheForTest()

	calls := 0
	lookPath := func(string) (string, error) { return "/usr/local/bin/bw", nil }
	execCommand := func(name string, args ...string) *exec.Cmd {
		if name == "bw" && len(args) > 0 && args[0] == "--version" {
			calls++
		}
		cmd := exec.Command("echo", "2026.2.0")
		return cmd
	}

	if err := checkBWInstalled(lookPath, execCommand, io.Discard); err != nil {
		t.Fatalf("first checkBWInstalled error = %v", err)
	}
	if err := checkBWInstalled(lookPath, execCommand, io.Discard); err != nil {
		t.Fatalf("second checkBWInstalled error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("bw --version calls = %d, want 1", calls)
	}
}
