//go:build darwin

package browser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func extractDMGArchive(archivePath, dest string, progressCb func(int, string)) error {
	mountDir, err := os.MkdirTemp("", "ant-browser-dmg-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(mountDir)

	progressCb(5, "正在挂载 DMG...")
	attach := exec.Command("hdiutil", "attach", "-nobrowse", "-readonly", "-mountpoint", mountDir, archivePath)
	if output, err := attach.CombinedOutput(); err != nil {
		return fmt.Errorf("挂载 DMG 失败: %s", strings.TrimSpace(string(output)))
	}
	defer func() {
		_ = exec.Command("hdiutil", "detach", mountDir, "-force").Run()
	}()

	appPath := ""
	_ = filepath.WalkDir(mountDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || path == mountDir || appPath != "" {
			return nil
		}
		if !entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".app") {
			return nil
		}
		if _, _, ok := FindCoreExecutableShallow(path); ok {
			appPath = path
		}
		return filepath.SkipDir
	})
	if appPath == "" {
		return fmt.Errorf("DMG 中未找到当前平台可用的 Chromium.app 或 Google Chrome.app")
	}

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	progressCb(40, "正在复制 macOS 浏览器内核...")
	target := filepath.Join(dest, filepath.Base(appPath))
	copyCmd := exec.Command("ditto", appPath, target)
	if output, err := copyCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("复制 macOS 浏览器内核失败: %s", strings.TrimSpace(string(output)))
	}
	if _, _, ok := FindCoreExecutableShallow(target); !ok {
		return fmt.Errorf("复制后的 macOS 浏览器内核不完整")
	}
	progressCb(100, "DMG 导入完成")
	return nil
}
