package browser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const extensionProfileSyncStateFile = ".ant-browser-extension-state.json"

type unpackedExtensionInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Path    string `json:"path"`
	Enabled bool   `json:"enabled"`
}

// SyncProfileExtensions 将应用配置同步成 Chromium profile 中持久化的 unpacked extension。
// 同步完成后，正常浏览器启动不再需要 --load-extension，因此不会重复触发 onInstalled。
func (m *Manager) SyncProfileExtensions(profileID string, chromeBinaryPath string, userDataDir string) error {
	desired := m.EnabledExtensionsForProfile(profileID)
	managedRoot := m.ResolveRelativePath(filepath.Join("data", extensionsRootDir))
	if extensionProfileSyncStateMatches(userDataDir, managedRoot, desired) {
		return nil
	}
	// 停用或移出作用域只通过启动白名单禁用，不卸载插件，避免清空插件数据。
	// 真正卸载只发生在用户明确执行“删除插件”时。
	if len(desired) > 0 {
		if err := syncProfileUnpackedExtensions(chromeBinaryPath, userDataDir, desired); err != nil {
			return err
		}
	}
	return writeExtensionProfileSyncState(userDataDir, desired)
}

func syncProfileUnpackedExtensions(chromeBinaryPath string, userDataDir string, desired []Extension) error {
	allowedDirs := make([]string, 0, len(desired))
	for _, extension := range desired {
		allowedDirs = append(allowedDirs, extension.InstallDir)
	}
	client, err := startExtensionMaintenanceBrowser(chromeBinaryPath, userDataDir, allowedDirs)
	if err != nil {
		return err
	}
	defer client.Close()

	current, err := client.List()
	if err != nil {
		return err
	}
	desiredByPath := make(map[string]Extension, len(desired))
	for _, extension := range desired {
		path := strings.TrimSpace(extension.InstallDir)
		if path == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(path, "manifest.json")); err != nil {
			return fmt.Errorf("插件 %s 的安装目录不可用: %w", extension.Name, err)
		}
		desiredByPath[normalizedExtensionPath(path)] = extension
	}

	currentByPath := make(map[string]unpackedExtensionInfo, len(current))
	for _, extension := range current {
		pathKey := normalizedExtensionPath(extension.Path)
		currentByPath[pathKey] = extension
	}

	for pathKey, extension := range desiredByPath {
		if installed, ok := currentByPath[pathKey]; ok && installed.Enabled && strings.TrimSpace(installed.Version) == strings.TrimSpace(extension.Version) {
			continue
		}
		if _, err := client.Load(extension.InstallDir); err != nil {
			return fmt.Errorf("向实例安装插件 %s 失败: %w", extension.Name, err)
		}
	}
	return nil
}

// UninstallProfileExtension 使用 Chromium 自身的卸载流程清理 profile 内的配置和存储。
func UninstallProfileExtension(chromeBinaryPath string, userDataDir string, installDir string) error {
	client, err := startExtensionMaintenanceBrowser(chromeBinaryPath, userDataDir, []string{installDir})
	if err != nil {
		return err
	}
	defer client.Close()

	// 被 --disable-extensions-except 排除后，Chromium 仍保留插件记录，但可能不再把它
	// 视为当前调试会话可卸载的 unpacked extension。先重新加载同一路径取得可管理 ID，
	// 再执行正式卸载，才能同时清理偏好设置和扩展存储。
	loadedID, err := client.Load(installDir)
	if err != nil {
		return fmt.Errorf("加载待卸载插件失败: %w", err)
	}
	if err := client.Uninstall(loadedID); err != nil {
		return fmt.Errorf("卸载插件 %s 失败: %w", loadedID, err)
	}
	return nil
}

func normalizedExtensionPath(path string) string {
	abs, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		abs = filepath.Clean(strings.TrimSpace(path))
	} else {
		abs = filepath.Clean(abs)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
		abs = resolved
	}
	if runtime.GOOS == "windows" {
		return strings.ToLower(abs)
	}
	return abs
}

func sameExtensionPath(left string, right string) bool {
	return normalizedExtensionPath(left) == normalizedExtensionPath(right)
}

type extensionProfileSyncState struct {
	Extensions []extensionProfileSyncStateItem `json:"extensions"`
}

type extensionProfileSyncStateItem struct {
	Path    string `json:"path"`
	Version string `json:"version"`
}

func extensionProfileSyncStateMatches(userDataDir string, managedRoot string, desired []Extension) bool {
	expected := buildExtensionProfileSyncState(desired)
	data, err := os.ReadFile(filepath.Join(userDataDir, extensionProfileSyncStateFile))
	if err != nil {
		return false
	}
	var stored extensionProfileSyncState
	if json.Unmarshal(data, &stored) != nil || !sameExtensionProfileSyncState(stored, expected) {
		return false
	}
	persisted := persistedManagedUnpackedExtensions(userDataDir, managedRoot)
	for _, item := range expected.Extensions {
		info, ok := persisted[normalizedExtensionPath(item.Path)]
		if !ok || len(info.DisableReasons) > 0 {
			return false
		}
	}
	return true
}

func writeExtensionProfileSyncState(userDataDir string, desired []Extension) error {
	state := buildExtensionProfileSyncState(desired)
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(userDataDir, 0o755); err != nil {
		return err
	}
	target := filepath.Join(userDataDir, extensionProfileSyncStateFile)
	temporary := target + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, target); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func buildExtensionProfileSyncState(desired []Extension) extensionProfileSyncState {
	state := extensionProfileSyncState{Extensions: make([]extensionProfileSyncStateItem, 0, len(desired))}
	for _, item := range desired {
		path := strings.TrimSpace(item.InstallDir)
		if path == "" {
			continue
		}
		state.Extensions = append(state.Extensions, extensionProfileSyncStateItem{
			Path:    normalizedExtensionPath(path),
			Version: strings.TrimSpace(item.Version),
		})
	}
	sort.Slice(state.Extensions, func(i, j int) bool { return state.Extensions[i].Path < state.Extensions[j].Path })
	return state
}

func sameExtensionProfileSyncState(left extensionProfileSyncState, right extensionProfileSyncState) bool {
	if len(left.Extensions) != len(right.Extensions) {
		return false
	}
	for index := range left.Extensions {
		if normalizedExtensionPath(left.Extensions[index].Path) != normalizedExtensionPath(right.Extensions[index].Path) ||
			strings.TrimSpace(left.Extensions[index].Version) != strings.TrimSpace(right.Extensions[index].Version) {
			return false
		}
	}
	return true
}
