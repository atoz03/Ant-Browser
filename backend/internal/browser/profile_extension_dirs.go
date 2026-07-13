package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type chromiumExtensionPreferences struct {
	Extensions struct {
		Settings map[string]struct {
			Location       int    `json:"location"`
			Path           string `json:"path"`
			DisableReasons []int  `json:"disable_reasons"`
		} `json:"settings"`
	} `json:"extensions"`
}

type persistedUnpackedExtension struct {
	Path           string
	DisableReasons []int
}

// LaunchExtensionDirsForProfile 返回应用托管插件和用户手动加载的开发者插件。
// 后者必须保留在 --disable-extensions-except 白名单中，否则 Chromium 会在下次启动时禁用它们。
func (m *Manager) LaunchExtensionDirsForProfile(profileID string, userDataDir string) []string {
	dirs := m.EnabledExtensionDirsForProfile(profileID)
	managedRoot := m.ResolveRelativePath(filepath.Join("data", extensionsRootDir))
	return appendUniqueExtensionDirs(dirs, persistedUnpackedExtensionDirs(userDataDir, managedRoot)...)
}

func persistedUnpackedExtensionDirs(userDataDir string, managedRoot string) []string {
	var prefs chromiumExtensionPreferences
	for _, name := range []string{"Secure Preferences", "Preferences"} {
		data, err := os.ReadFile(filepath.Join(userDataDir, "Default", name))
		if err != nil || json.Unmarshal(data, &prefs) != nil {
			continue
		}
		if len(prefs.Extensions.Settings) > 0 {
			break
		}
	}

	dirs := make([]string, 0)
	for _, item := range prefs.Extensions.Settings {
		// Chromium 的 location=4 表示用户通过开发者模式加载的 unpacked extension。
		if item.Location != 4 {
			continue
		}
		dir := strings.TrimSpace(item.Path)
		if dir == "" || isPathWithin(dir, managedRoot) {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err == nil {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

// ProfileReferencesExtensionPath 判断 Chromium profile 是否仍记录指定插件目录。
// 同时检查 Preferences 和 Secure Preferences，以覆盖旧的命令行加载记录和新的持久化记录。
func ProfileReferencesExtensionPath(userDataDir string, installDir string) bool {
	for _, name := range []string{"Secure Preferences", "Preferences"} {
		var prefs chromiumExtensionPreferences
		data, err := os.ReadFile(filepath.Join(userDataDir, "Default", name))
		if err != nil || json.Unmarshal(data, &prefs) != nil {
			continue
		}
		for _, item := range prefs.Extensions.Settings {
			if sameExtensionPath(item.Path, installDir) {
				return true
			}
		}
	}
	return false
}

func persistedManagedUnpackedExtensions(userDataDir string, managedRoot string) map[string]persistedUnpackedExtension {
	items := make(map[string]persistedUnpackedExtension)
	for _, name := range []string{"Secure Preferences", "Preferences"} {
		var prefs chromiumExtensionPreferences
		data, err := os.ReadFile(filepath.Join(userDataDir, "Default", name))
		if err != nil || json.Unmarshal(data, &prefs) != nil {
			continue
		}
		for _, item := range prefs.Extensions.Settings {
			if item.Location == 4 && isPathWithin(item.Path, managedRoot) {
				items[normalizedExtensionPath(item.Path)] = persistedUnpackedExtension{
					Path:           item.Path,
					DisableReasons: append([]int(nil), item.DisableReasons...),
				}
			}
		}
	}
	return items
}

func appendUniqueExtensionDirs(base []string, extra ...string) []string {
	result := make([]string, 0, len(base)+len(extra))
	seen := make(map[string]struct{}, len(base)+len(extra))
	for _, value := range append(append([]string{}, base...), extra...) {
		dir := strings.TrimSpace(value)
		if dir == "" {
			continue
		}
		key := normalizedExtensionPath(dir)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, dir)
	}
	return result
}

func isPathWithin(path string, root string) bool {
	pathAbs, pathErr := filepath.Abs(path)
	rootAbs, rootErr := filepath.Abs(root)
	if pathErr != nil || rootErr != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
