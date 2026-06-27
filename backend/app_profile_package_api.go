package backend

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ant-chrome/backend/internal/browser"

	"github.com/google/uuid"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const profilePackageFormat = "ant-chrome-profile-package"

type ProfilePackageManifest struct {
	Format       string `json:"format"`
	Version      int    `json:"version"`
	ExportedAt   string `json:"exportedAt"`
	ProfileCount int    `json:"profileCount"`
}

type ProfilePackageExportResult struct {
	Cancelled    bool   `json:"cancelled"`
	ZipPath      string `json:"zipPath"`
	ProfileCount int    `json:"profileCount"`
	FileCount    int    `json:"fileCount"`
	Message      string `json:"message"`
}

type ProfilePackageImportResult struct {
	Cancelled       bool              `json:"cancelled"`
	ImportedCount   int               `json:"importedCount"`
	ProfileMappings map[string]string `json:"profileMappings"`
	Message         string            `json:"message"`
}

// BrowserProfilePackageExport 导出选中的实例配置和浏览器用户数据目录。
func (a *App) BrowserProfilePackageExport(profileIds []string) (ProfilePackageExportResult, error) {
	a.maintenanceMu.Lock()
	defer a.maintenanceMu.Unlock()

	ids := normalizeProfilePackageIDs(profileIds)
	if len(ids) == 0 {
		return ProfilePackageExportResult{}, fmt.Errorf("请选择要导出的实例")
	}
	if a.ctx == nil {
		return ProfilePackageExportResult{}, fmt.Errorf("应用上下文未初始化")
	}

	profiles, err := a.collectProfilesForPackage(ids)
	if err != nil {
		return ProfilePackageExportResult{}, err
	}

	defaultName := fmt.Sprintf("ant-chrome-profile-package-%s.zip", time.Now().Format("20060102-150405"))
	savePath, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           "导出实例",
		DefaultFilename: defaultName,
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "ZIP 文件 (*.zip)", Pattern: "*.zip"},
		},
	})
	if err != nil {
		return ProfilePackageExportResult{}, fmt.Errorf("打开保存对话框失败: %w", err)
	}
	if strings.TrimSpace(savePath) == "" {
		return ProfilePackageExportResult{Cancelled: true, Message: "已取消导出"}, nil
	}
	savePath = ensureZipSuffix(savePath)

	fileCount, err := a.writeProfilePackage(savePath, profiles)
	if err != nil {
		return ProfilePackageExportResult{}, err
	}
	return ProfilePackageExportResult{
		Cancelled:    false,
		ZipPath:      savePath,
		ProfileCount: len(profiles),
		FileCount:    fileCount,
		Message:      "导出完成",
	}, nil
}

// BrowserProfilePackageImport 导入实例包，冲突时始终生成新实例和新目录。
func (a *App) BrowserProfilePackageImport() (ProfilePackageImportResult, error) {
	a.maintenanceMu.Lock()
	defer a.maintenanceMu.Unlock()

	if a.ctx == nil {
		return ProfilePackageImportResult{}, fmt.Errorf("应用上下文未初始化")
	}
	zipPath, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "导入实例",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "ZIP 文件 (*.zip)", Pattern: "*.zip"},
		},
	})
	if err != nil {
		return ProfilePackageImportResult{}, fmt.Errorf("打开文件对话框失败: %w", err)
	}
	if strings.TrimSpace(zipPath) == "" {
		return ProfilePackageImportResult{Cancelled: true, Message: "已取消导入"}, nil
	}
	return a.importProfilePackageFromPath(zipPath)
}

func (a *App) collectProfilesForPackage(profileIds []string) ([]browser.Profile, error) {
	a.browserMgr.InitData()
	a.browserMgr.Mutex.Lock()
	defer a.browserMgr.Mutex.Unlock()

	profiles := make([]browser.Profile, 0, len(profileIds))
	missing := make([]string, 0)
	running := make([]string, 0)
	for _, id := range profileIds {
		profile := a.browserMgr.Profiles[id]
		if profile == nil {
			missing = append(missing, id)
			continue
		}
		if profile.Running {
			running = append(running, profile.ProfileName)
			continue
		}
		copyProfile := *profile
		copyProfile.LaunchCode = ""
		copyProfile.Running = false
		copyProfile.DebugPort = 0
		copyProfile.DebugReady = false
		copyProfile.Pid = 0
		copyProfile.RuntimeWarning = ""
		copyProfile.LastError = ""
		a.prepareProfileProxyForPackage(&copyProfile)
		profiles = append(profiles, copyProfile)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("实例不存在: %s", strings.Join(missing, ", "))
	}
	if len(running) > 0 {
		return nil, fmt.Errorf("请先停止实例再导出: %s", strings.Join(running, ", "))
	}
	return profiles, nil
}

func (a *App) writeProfilePackage(zipPath string, profiles []browser.Profile) (int, error) {
	if err := os.MkdirAll(filepath.Dir(zipPath), 0o755); err != nil {
		return 0, fmt.Errorf("创建导出目录失败: %w", err)
	}
	tmpPath := zipPath + ".tmp"
	_ = os.Remove(tmpPath)
	out, err := os.Create(tmpPath)
	if err != nil {
		return 0, fmt.Errorf("创建导出文件失败: %w", err)
	}
	zipWriter := zip.NewWriter(out)
	fileCount := 0

	writeErr := func() error {
		manifest := ProfilePackageManifest{
			Format:       profilePackageFormat,
			Version:      1,
			ExportedAt:   time.Now().Format(time.RFC3339),
			ProfileCount: len(profiles),
		}
		if err := writeProfilePackageJSON(zipWriter, "manifest.json", manifest); err != nil {
			return err
		}
		fileCount++
		if err := writeProfilePackageJSON(zipWriter, "profiles.json", profiles); err != nil {
			return err
		}
		fileCount++
		for i := range profiles {
			profile := &profiles[i]
			userDataDir := a.browserMgr.ResolveUserDataDir(profile)
			if _, err := os.Stat(userDataDir); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return fmt.Errorf("读取用户数据目录失败: %w", err)
			}
			added, err := writeProfilePackageDir(zipWriter, userDataDir, "user-data/"+profile.ProfileId)
			if err != nil {
				return fmt.Errorf("打包用户数据失败 [%s]: %w", profile.ProfileName, err)
			}
			fileCount += added
		}
		return nil
	}()

	closeZipErr := zipWriter.Close()
	closeFileErr := out.Close()
	if writeErr != nil {
		_ = os.Remove(tmpPath)
		return 0, writeErr
	}
	if closeZipErr != nil {
		_ = os.Remove(tmpPath)
		return 0, closeZipErr
	}
	if closeFileErr != nil {
		_ = os.Remove(tmpPath)
		return 0, closeFileErr
	}
	if err := os.Rename(tmpPath, zipPath); err != nil {
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("保存导出文件失败: %w", err)
	}
	return fileCount, nil
}

func (a *App) importProfilePackageFromPath(zipPath string) (ProfilePackageImportResult, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return ProfilePackageImportResult{}, fmt.Errorf("打开实例包失败: %w", err)
	}
	defer reader.Close()

	var manifest ProfilePackageManifest
	if err := readProfilePackageJSON(reader.File, "manifest.json", &manifest); err != nil {
		return ProfilePackageImportResult{}, err
	}
	if manifest.Format != profilePackageFormat || manifest.Version != 1 {
		return ProfilePackageImportResult{}, fmt.Errorf("不支持的实例包格式")
	}
	var profiles []browser.Profile
	if err := readProfilePackageJSON(reader.File, "profiles.json", &profiles); err != nil {
		return ProfilePackageImportResult{}, err
	}
	if len(profiles) == 0 {
		return ProfilePackageImportResult{}, fmt.Errorf("实例包为空")
	}

	a.browserMgr.InitData()
	now := time.Now().Format(time.RFC3339)
	mappings := make(map[string]string, len(profiles))
	prepared := make([]browser.Profile, 0, len(profiles))
	for _, source := range profiles {
		oldID := strings.TrimSpace(source.ProfileId)
		if oldID == "" {
			oldID = uuid.NewString()
		}
		newID := uuid.NewString()
		source.ProfileId = newID
		source.ProfileName = buildImportedProfileName(source.ProfileName)
		source.UserDataDir = newID
		source.Running = false
		source.DebugPort = 0
		source.DebugReady = false
		source.Pid = 0
		source.RuntimeWarning = ""
		source.LastError = ""
		source.LaunchCode = ""
		source.CreatedAt = now
		source.UpdatedAt = now
		source.DeletedAt = ""
		a.applyImportedProfileProxyByName(&source)
		prepared = append(prepared, source)
		mappings[oldID] = newID
	}

	for _, profile := range prepared {
		if err := a.extractProfileUserData(reader.File, mappings, profile.ProfileId); err != nil {
			return ProfilePackageImportResult{}, err
		}
	}
	a.browserMgr.Mutex.Lock()
	for i := range prepared {
		profile := &prepared[i]
		a.browserMgr.Profiles[profile.ProfileId] = profile
		if a.launchCodeSvc != nil {
			if code, err := a.launchCodeSvc.EnsureCode(profile.ProfileId); err == nil {
				profile.LaunchCode = code
			}
		}
	}
	a.browserMgr.Mutex.Unlock()
	if err := a.browserMgr.SaveProfiles(); err != nil {
		return ProfilePackageImportResult{}, err
	}

	return ProfilePackageImportResult{
		Cancelled:       false,
		ImportedCount:   len(prepared),
		ProfileMappings: mappings,
		Message:         "导入完成",
	}, nil
}

func (a *App) extractProfileUserData(files []*zip.File, mappings map[string]string, newProfileID string) error {
	oldProfileID := ""
	for oldID, mappedID := range mappings {
		if mappedID == newProfileID {
			oldProfileID = oldID
			break
		}
	}
	if oldProfileID == "" {
		return fmt.Errorf("实例映射不存在: %s", newProfileID)
	}
	profile := &browser.Profile{ProfileId: newProfileID, UserDataDir: newProfileID}
	destDir := a.browserMgr.ResolveUserDataDir(profile)
	if err := os.RemoveAll(destDir); err != nil {
		return fmt.Errorf("清理用户数据目录失败: %w", err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("创建用户数据目录失败: %w", err)
	}
	prefix := "user-data/" + oldProfileID + "/"
	for _, file := range files {
		name := filepath.ToSlash(file.Name)
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		rel := strings.TrimPrefix(name, prefix)
		if rel == "" {
			continue
		}
		if err := extractProfilePackageFile(file, destDir, rel); err != nil {
			return err
		}
	}
	return nil
}

func writeProfilePackageJSON(zipWriter *zip.Writer, name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	writer, err := zipWriter.Create(name)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

func writeProfilePackageDir(zipWriter *zip.Writer, srcDir string, destPrefix string) (int, error) {
	count := 0
	err := filepath.WalkDir(srcDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		zipName := filepath.ToSlash(filepath.Join(destPrefix, rel))
		if entry.IsDir() {
			_, err := zipWriter.Create(strings.TrimSuffix(zipName, "/") + "/")
			return err
		}
		writer, err := zipWriter.Create(zipName)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		count++
		return nil
	})
	return count, err
}

func readProfilePackageJSON(files []*zip.File, name string, target any) error {
	for _, file := range files {
		if filepath.ToSlash(file.Name) != name {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			return err
		}
		defer reader.Close()
		return json.NewDecoder(reader).Decode(target)
	}
	return fmt.Errorf("实例包缺少 %s", name)
}

func extractProfilePackageFile(file *zip.File, destDir string, rel string) error {
	cleanRel := filepath.Clean(filepath.FromSlash(rel))
	if cleanRel == "." || strings.HasPrefix(cleanRel, "..") || filepath.IsAbs(cleanRel) {
		return fmt.Errorf("非法路径: %s", rel)
	}
	target := filepath.Join(destDir, cleanRel)
	cleanDest := filepath.Clean(destDir)
	cleanTarget := filepath.Clean(target)
	if cleanTarget != cleanDest && !strings.HasPrefix(cleanTarget, cleanDest+string(os.PathSeparator)) {
		return fmt.Errorf("非法路径: %s", rel)
	}
	if file.FileInfo().IsDir() {
		return os.MkdirAll(target, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	reader, err := file.Open()
	if err != nil {
		return err
	}
	defer reader.Close()
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, reader)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func normalizeProfilePackageIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func ensureZipSuffix(path string) string {
	if strings.EqualFold(filepath.Ext(path), ".zip") {
		return path
	}
	return path + ".zip"
}

func buildImportedProfileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "导入实例"
	}
	return name + "（导入）"
}

func (a *App) prepareProfileProxyForPackage(profile *browser.Profile) {
	if profile == nil {
		return
	}
	proxyName := strings.TrimSpace(profile.ProxyBindName)
	if proxyName == "" {
		if proxy, ok := a.browserMgr.GetProxyByID(profile.ProxyId); ok {
			proxyName = strings.TrimSpace(proxy.ProxyName)
		}
	}
	profile.ProxyId = ""
	profile.ProxyConfig = ""
	profile.ProxyBindSourceID = ""
	profile.ProxyBindSourceURL = ""
	profile.ProxyBindName = proxyName
	profile.ProxyBindUpdatedAt = ""
}

func (a *App) applyImportedProfileProxyByName(profile *browser.Profile) {
	if profile == nil {
		return
	}
	proxyName := strings.TrimSpace(profile.ProxyBindName)
	profile.ProxyId = ""
	profile.ProxyConfig = ""
	profile.ProxyBindSourceID = ""
	profile.ProxyBindSourceURL = ""
	profile.ProxyBindUpdatedAt = ""
	if proxyName == "" {
		profile.ProxyBindName = ""
		return
	}
	if proxy, ok := a.findUniqueProxyByName(proxyName); ok {
		browser.BindProfileToProxy(profile, proxy, true)
		return
	}
	profile.ProxyBindName = ""
}

func (a *App) findUniqueProxyByName(proxyName string) (browser.Proxy, bool) {
	target := strings.ToLower(strings.TrimSpace(proxyName))
	if target == "" {
		return browser.Proxy{}, false
	}
	proxies := browser.ListProxiesWithFallback(a.browserMgr.ProxyDAO, a.config.Browser.Proxies)
	var hit browser.Proxy
	matched := 0
	for _, proxy := range proxies {
		if strings.ToLower(strings.TrimSpace(proxy.ProxyName)) != target {
			continue
		}
		hit = proxy
		matched++
		if matched > 1 {
			return browser.Proxy{}, false
		}
	}
	return hit, matched == 1
}
