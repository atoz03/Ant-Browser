//go:build !windows
// +build !windows

package backend

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func findBrowserUserDataProcessesOS(userDataDir string) ([]browserUserDataProcess, error) {
	target, err := filepath.Abs(strings.TrimSpace(userDataDir))
	if err != nil || target == "" {
		return nil, err
	}
	output, err := exec.Command("ps", "-ww", "-axo", "pid=,command=").Output()
	if err != nil {
		return nil, fmt.Errorf("查询浏览器进程失败: %w", err)
	}
	return parseBrowserProcessesFromPS(string(output), target), nil
}

func terminateBrowserUserDataProcessOS(pid int, timeout time.Duration) error {
	if pid <= 0 {
		return nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := process.Signal(syscall.SIGTERM); err != nil && !isProcessAlreadyFinished(err) {
		return err
	}
	if waitForUnixProcessExit(pid, timeout) {
		return nil
	}
	if err := process.Kill(); err != nil && !isProcessAlreadyFinished(err) {
		return err
	}
	if waitForUnixProcessExit(pid, 2*time.Second) {
		return nil
	}
	return fmt.Errorf("进程 %d 未在超时内退出", pid)
}

func parseBrowserProcessesFromPS(output string, targetUserDataDir string) []browserUserDataProcess {
	target := filepath.Clean(targetUserDataDir)
	items := make([]browserUserDataProcess, 0)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		separator := strings.IndexAny(line, " \t")
		if separator <= 0 {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(line[:separator]))
		if err != nil || pid <= 0 || pid == os.Getpid() {
			continue
		}
		commandLine := strings.TrimSpace(line[separator:])
		userDataDir := commandLineSwitchValue(commandLine, "--user-data-dir")
		if userDataDir == "" {
			continue
		}
		resolved, err := filepath.Abs(userDataDir)
		if err != nil || filepath.Clean(resolved) != target {
			continue
		}
		items = append(items, browserUserDataProcess{
			PID:         pid,
			DebugPort:   parseRemoteDebuggingPort(commandLine),
			CommandLine: commandLine,
		})
	}
	return items
}

func commandLineSwitchValue(commandLine string, name string) string {
	prefix := name + "="
	index := strings.Index(commandLine, prefix)
	if index < 0 {
		return ""
	}
	value := strings.TrimSpace(commandLine[index+len(prefix):])
	if value == "" {
		return ""
	}
	if value[0] == '"' || value[0] == '\'' {
		quote := value[0]
		if end := strings.IndexByte(value[1:], quote); end >= 0 {
			return value[1 : end+1]
		}
		return strings.Trim(value, string(quote))
	}
	if end := strings.Index(value, " --"); end >= 0 {
		value = value[:end]
	}
	return strings.TrimSpace(value)
}

func waitForUnixProcessExit(pid int, timeout time.Duration) bool {
	if timeout <= 0 {
		timeout = time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		process, err := os.FindProcess(pid)
		if err != nil || process.Signal(syscall.Signal(0)) != nil {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}
