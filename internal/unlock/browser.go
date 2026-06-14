package unlock

import (
	"fmt"
	"os/exec"
	"runtime"
)

type BrowserOpener func(url string) error

var openBrowser BrowserOpener = defaultOpenBrowser

func defaultOpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("auto-open browser not supported on %s", runtime.GOOS)
	}
	return cmd.Start()
}
