//go:build !windows

package browser

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

func startExtensionMaintenanceBrowser(chromeBinaryPath string, userDataDir string, allowedDirs []string) (*extensionMaintenanceBrowser, error) {
	toChromeRead, toChromeWrite, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	fromChromeRead, fromChromeWrite, err := os.Pipe()
	if err != nil {
		toChromeRead.Close()
		toChromeWrite.Close()
		return nil, err
	}
	cleanup := func() {
		toChromeRead.Close()
		toChromeWrite.Close()
		fromChromeRead.Close()
		fromChromeWrite.Close()
	}

	args := append(extensionMaintenanceArgs(userDataDir, allowedDirs), "about:blank")
	cmd := exec.Command(chromeBinaryPath, args...)
	cmd.Stdout = io.Discard
	cmd.ExtraFiles = []*os.File{toChromeRead, fromChromeWrite}
	client := newExtensionMaintenanceBrowser(cmd, toChromeWrite, fromChromeRead)
	if err := cmd.Start(); err != nil {
		cleanup()
		return nil, fmt.Errorf("启动插件维护进程失败: %w", err)
	}
	toChromeRead.Close()
	fromChromeWrite.Close()
	if err := client.ensureReady(); err != nil {
		return nil, err
	}
	return client, nil
}
