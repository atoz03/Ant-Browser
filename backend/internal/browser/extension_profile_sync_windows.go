//go:build windows

package browser

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
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

	inputHandle := toChromeRead.Fd()
	outputHandle := fromChromeWrite.Fd()
	args := extensionMaintenanceArgs(userDataDir, allowedDirs)
	args = append(args, "--remote-debugging-io-pipes="+strconv.FormatUint(uint64(inputHandle), 10)+","+strconv.FormatUint(uint64(outputHandle), 10))
	args = append(args, "about:blank")
	cmd := exec.Command(chromeBinaryPath, args...)
	cmd.Stdout = io.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
		AdditionalInheritedHandles: []syscall.Handle{
			syscall.Handle(inputHandle),
			syscall.Handle(outputHandle),
		},
	}
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
