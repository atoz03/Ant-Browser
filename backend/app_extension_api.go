package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/config"
	"ant-chrome/backend/internal/logger"
	"ant-chrome/backend/internal/proxy"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type BrowserExtension = browser.Extension
type BrowserExtensionLookupResult = browser.ExtensionLookupResult
type BrowserProfileExtensionSettings = browser.ProfileExtensionSettings
type BrowserExtensionProfileScope = browser.ExtensionProfileScope

type BrowserExtensionWebStoreRequest struct {
	Query       string `json:"query"`
	UseProxy    bool   `json:"useProxy"`
	ProxyConfig string `json:"proxyConfig"`
}

type BrowserExtensionManualInstallGuide struct {
	ExtensionID string `json:"extensionId"`
	StoreURL    string `json:"storeUrl"`
	DownloadURL string `json:"downloadUrl"`
	DownloadDir string `json:"downloadDir"`
	FileName    string `json:"fileName"`
}

type BrowserExtensionManualDownloadFile struct {
	FileName  string `json:"fileName"`
	FilePath  string `json:"filePath"`
	SizeBytes int64  `json:"sizeBytes"`
	UpdatedAt string `json:"updatedAt"`
}

func (a *App) BrowserExtensionList() ([]BrowserExtension, error) {
	if a.browserMgr == nil || a.browserMgr.ExtensionDAO == nil {
		return []BrowserExtension{}, nil
	}
	items, err := a.browserMgr.ListExtensions()
	if err != nil {
		return nil, err
	}
	for index := range items {
		items[index] = a.populateExtensionScope(items[index])
	}
	return items, nil
}

func (a *App) BrowserExtensionLookup(query string) (BrowserExtensionLookupResult, error) {
	return a.BrowserExtensionLookupWithProxy(BrowserExtensionWebStoreRequest{Query: query})
}

func (a *App) BrowserExtensionLookupWithProxy(input BrowserExtensionWebStoreRequest) (BrowserExtensionLookupResult, error) {
	if a.browserMgr == nil {
		return BrowserExtensionLookupResult{}, fmt.Errorf("浏览器管理器未初始化")
	}
	client, err := a.extensionDownloadHTTPClient(input.UseProxy, input.ProxyConfig)
	if err != nil {
		return BrowserExtensionLookupResult{}, fmt.Errorf("下载代理配置错误: %w", err)
	}
	return a.browserMgr.LookupExtensionWithHTTPClient(input.Query, client)
}

func (a *App) BrowserExtensionInstall(query string) (BrowserExtension, error) {
	return a.BrowserExtensionInstallWithProxy(BrowserExtensionWebStoreRequest{Query: query})
}

func (a *App) BrowserExtensionInstallWithProxy(input BrowserExtensionWebStoreRequest) (BrowserExtension, error) {
	a.maintenanceMu.Lock()
	defer a.maintenanceMu.Unlock()

	if a.ctx == nil {
		return BrowserExtension{}, fmt.Errorf("应用上下文未初始化")
	}
	if a.browserMgr == nil {
		return BrowserExtension{}, fmt.Errorf("浏览器管理器未初始化")
	}
	client, err := a.extensionDownloadHTTPClient(input.UseProxy, input.ProxyConfig)
	if err != nil {
		return BrowserExtension{}, fmt.Errorf("下载代理配置错误: %w", err)
	}
	return a.browserMgr.InstallExtensionFromWebStoreWithHTTPClient(a.ctx, input.Query, client)
}

func (a *App) BrowserExtensionManualInstallGuide(query string) (BrowserExtensionManualInstallGuide, error) {
	extensionID := browser.NormalizeExtensionID(query)
	if extensionID == "" {
		return BrowserExtensionManualInstallGuide{}, fmt.Errorf("请输入 Chrome 插件 ID 或 Chrome Web Store 链接")
	}
	downloadDir := a.extensionManualDownloadDir()
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		return BrowserExtensionManualInstallGuide{}, fmt.Errorf("创建手动下载目录失败: %w", err)
	}
	return BrowserExtensionManualInstallGuide{
		ExtensionID: extensionID,
		StoreURL:    browser.BuildChromeWebStoreURL(extensionID),
		DownloadURL: browser.BuildChromeExtensionDownloadURL(extensionID),
		DownloadDir: downloadDir,
		FileName:    extensionID + ".crx",
	}, nil
}

func (a *App) BrowserExtensionOpenManualDownloadDir() error {
	downloadDir := a.extensionManualDownloadDir()
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		return fmt.Errorf("创建手动下载目录失败: %w", err)
	}
	absPath, err := filepath.Abs(downloadDir)
	if err != nil {
		return err
	}
	return openPathInFileManager(absPath)
}

func (a *App) BrowserExtensionListManualDownloadFiles() ([]BrowserExtensionManualDownloadFile, error) {
	downloadDir := a.extensionManualDownloadDir()
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建手动下载目录失败: %w", err)
	}
	entries, err := os.ReadDir(downloadDir)
	if err != nil {
		return nil, fmt.Errorf("读取手动下载目录失败: %w", err)
	}
	files := make([]BrowserExtensionManualDownloadFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		lowerName := strings.ToLower(name)
		if !strings.HasSuffix(lowerName, ".crx") && !strings.HasSuffix(lowerName, ".zip") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, BrowserExtensionManualDownloadFile{
			FileName:  name,
			FilePath:  filepath.Join(downloadDir, name),
			SizeBytes: info.Size(),
			UpdatedAt: info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}
	return files, nil
}

func (a *App) BrowserExtensionInstallManualDownloadFile(fileName string) (BrowserExtension, error) {
	a.maintenanceMu.Lock()
	defer a.maintenanceMu.Unlock()

	if a.browserMgr == nil {
		return BrowserExtension{}, fmt.Errorf("浏览器管理器未初始化")
	}
	path, err := a.resolveManualDownloadFile(fileName)
	if err != nil {
		return BrowserExtension{}, err
	}
	return a.browserMgr.InstallExtensionPackageFile(path)
}

func (a *App) extensionManualDownloadDir() string {
	return a.resolveAppPath(filepath.ToSlash(filepath.Join("data", "extensions", "manual-downloads")))
}

func (a *App) resolveManualDownloadFile(fileName string) (string, error) {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return "", fmt.Errorf("请选择要导入的插件包")
	}
	baseDir, err := filepath.Abs(a.extensionManualDownloadDir())
	if err != nil {
		return "", err
	}
	cleanName := filepath.Clean(filepath.FromSlash(fileName))
	if cleanName == "." || cleanName == "" || filepath.IsAbs(cleanName) || strings.HasPrefix(cleanName, ".."+string(os.PathSeparator)) || cleanName == ".." {
		return "", fmt.Errorf("插件包路径无效")
	}
	lowerName := strings.ToLower(cleanName)
	if !strings.HasSuffix(lowerName, ".crx") && !strings.HasSuffix(lowerName, ".zip") {
		return "", fmt.Errorf("只支持导入 .crx 或 .zip 插件包")
	}
	fullPath, err := filepath.Abs(filepath.Join(baseDir, cleanName))
	if err != nil {
		return "", err
	}
	if fullPath != baseDir && !strings.HasPrefix(fullPath, baseDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("插件包路径越界")
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		return "", fmt.Errorf("插件包不存在: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("请选择插件包文件，不是目录")
	}
	return fullPath, nil
}

func (a *App) extensionDownloadHTTPClient(useProxy bool, proxyConfig string) (*http.Client, error) {
	proxyConfig = strings.TrimSpace(proxyConfig)
	log := logger.New("Extension")
	if useProxy && proxyConfig == "" {
		return nil, fmt.Errorf("已启用下载代理，但代理配置为空，请重新选择代理节点")
	}
	if proxyConfig == "" || strings.EqualFold(proxyConfig, "direct://") {
		log.Info("Chrome 插件下载使用直连")
		client, _, err := proxyCoreHTTPClient(browser.ExtensionDownloadTimeout(), "")
		return client, err
	}
	proxies := a.getLatestProxies()
	connectorType := config.BrowserConnectorXray
	if a != nil && a.config != nil {
		connectorType = a.config.Browser.DefaultConnectorType
	}
	log.Info("Chrome 插件下载使用代理", logger.F("connector", connectorType), logger.F("proxy_prefix", proxyConfigLogPrefix(proxyConfig)))
	return proxy.BuildProxyHTTPClient(proxyConfig, "", proxies, a.xrayMgr, a.singboxMgr, a.clashMgr, connectorType, browser.ExtensionDownloadTimeout())
}

func proxyConfigLogPrefix(proxyConfig string) string {
	proxyConfig = strings.TrimSpace(proxyConfig)
	if len(proxyConfig) <= 24 {
		return proxyConfig
	}
	return proxyConfig[:24]
}

func (a *App) BrowserExtensionInstallLocalFile() (BrowserExtension, error) {
	a.maintenanceMu.Lock()
	defer a.maintenanceMu.Unlock()

	if a.ctx == nil {
		return BrowserExtension{}, fmt.Errorf("应用上下文未初始化")
	}
	if a.browserMgr == nil {
		return BrowserExtension{}, fmt.Errorf("浏览器管理器未初始化")
	}
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "选择 Chrome 插件包",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Chrome 插件包 (*.crx;*.zip)", Pattern: "*.crx;*.zip"},
		},
	})
	if err != nil {
		return BrowserExtension{}, fmt.Errorf("打开文件选择框失败: %w", err)
	}
	if strings.TrimSpace(path) == "" {
		return BrowserExtension{}, fmt.Errorf("已取消选择")
	}
	return a.browserMgr.InstallExtensionPackageFile(path)
}

func (a *App) BrowserExtensionInstallLocalDirectory() (BrowserExtension, error) {
	a.maintenanceMu.Lock()
	defer a.maintenanceMu.Unlock()

	if a.ctx == nil {
		return BrowserExtension{}, fmt.Errorf("应用上下文未初始化")
	}
	if a.browserMgr == nil {
		return BrowserExtension{}, fmt.Errorf("浏览器管理器未初始化")
	}
	path, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "选择已解压插件目录"})
	if err != nil {
		return BrowserExtension{}, fmt.Errorf("打开目录选择框失败: %w", err)
	}
	if strings.TrimSpace(path) == "" {
		return BrowserExtension{}, fmt.Errorf("已取消选择")
	}
	return a.browserMgr.InstallExtensionDirectory(path)
}

func (a *App) BrowserExtensionSetEnabled(extensionID string, enabled bool) (BrowserExtension, error) {
	if a.browserMgr == nil || a.browserMgr.ExtensionDAO == nil {
		return BrowserExtension{}, fmt.Errorf("插件管理器未初始化")
	}
	extensionID = strings.TrimSpace(extensionID)
	if extensionID == "" {
		return BrowserExtension{}, fmt.Errorf("插件 ID 不能为空")
	}
	if err := a.browserMgr.ExtensionDAO.SetEnabled(extensionID, enabled); err != nil {
		return BrowserExtension{}, err
	}
	extension, err := a.browserMgr.ExtensionDAO.Get(extensionID)
	if err != nil {
		return BrowserExtension{}, err
	}
	return a.populateExtensionScope(extension), nil
}

func (a *App) populateExtensionScope(extension BrowserExtension) BrowserExtension {
	if a.browserMgr == nil || a.browserMgr.ExtensionDAO == nil {
		return extension
	}
	scope, err := a.browserMgr.ExtensionDAO.GetExtensionProfileScope(extension.ExtensionID)
	if err != nil {
		return extension
	}
	extension.ScopeRestricted = scope.Restricted
	activeProfiles := make(map[string]struct{})
	for _, profile := range a.browserMgr.List() {
		activeProfiles[profile.ProfileId] = struct{}{}
	}
	for _, profileID := range scope.ProfileIDs {
		if _, active := activeProfiles[profileID]; active {
			extension.ScopeProfileCount++
		}
	}
	return extension
}

func (a *App) BrowserExtensionDelete(extensionID string) error {
	a.maintenanceMu.Lock()
	defer a.maintenanceMu.Unlock()

	if a.browserMgr == nil || a.browserMgr.ExtensionDAO == nil {
		return fmt.Errorf("插件管理器未初始化")
	}
	extensionID = strings.TrimSpace(extensionID)
	if extensionID == "" {
		return fmt.Errorf("插件 ID 不能为空")
	}
	extension, err := a.browserMgr.ExtensionDAO.Get(extensionID)
	if err != nil {
		return err
	}
	installDir, err := a.resolveBrowserExtensionInstallDir(a.browserMgr.ManagedExtensionInstallDir(extension.ExtensionID))
	if err != nil {
		return err
	}
	if err := a.uninstallBrowserExtensionFromProfiles(installDir); err != nil {
		return err
	}
	if err := a.removeBrowserExtensionFiles(extension, installDir); err != nil {
		return err
	}
	if err := a.browserMgr.ExtensionDAO.Delete(extensionID); err != nil {
		return err
	}
	return nil
}

func (a *App) uninstallBrowserExtensionFromProfiles(installDir string) error {
	profiles := append(a.browserMgr.List(), a.browserMgr.ListDeleted()...)
	seen := make(map[string]struct{}, len(profiles))
	cleanupProfiles := make([]BrowserProfile, 0)
	busyProfiles := make([]string, 0)
	for _, profile := range profiles {
		if _, exists := seen[profile.ProfileId]; exists {
			continue
		}
		seen[profile.ProfileId] = struct{}{}
		userDataDir := a.browserMgr.ResolveUserDataDir(&profile)
		if !browser.ProfileReferencesExtensionPath(userDataDir, installDir) {
			continue
		}
		if profile.Running {
			busyProfiles = append(busyProfiles, profile.ProfileName)
			continue
		}
		if detection, ok := detectBrowserRuntimeByActivePort(userDataDir); ok && detection.PID > 0 {
			busyProfiles = append(busyProfiles, profile.ProfileName)
			continue
		}
		cleanupProfiles = append(cleanupProfiles, profile)
	}
	if len(busyProfiles) > 0 {
		return fmt.Errorf("插件仍被运行中的实例使用，请先关闭后再删除：%s", strings.Join(busyProfiles, "、"))
	}

	for _, profile := range cleanupProfiles {
		chromeBinaryPath, err := a.browserMgr.ResolveChromeBinary(&profile)
		if err != nil {
			return fmt.Errorf("清理实例 %s 的插件数据失败：%w", profile.ProfileName, err)
		}
		userDataDir := a.browserMgr.ResolveUserDataDir(&profile)
		if err := browser.UninstallProfileExtension(chromeBinaryPath, userDataDir, installDir); err != nil {
			return fmt.Errorf("清理实例 %s 的插件数据失败：%w", profile.ProfileName, err)
		}
		if browser.ProfileReferencesExtensionPath(userDataDir, installDir) {
			return fmt.Errorf("实例 %s 仍残留插件配置，删除已中止", profile.ProfileName)
		}
	}
	return nil
}

func (a *App) resolveBrowserExtensionInstallDir(installDir string) (string, error) {
	installDir = strings.TrimSpace(installDir)
	if installDir == "" {
		return "", nil
	}
	target, err := filepath.Abs(installDir)
	if err != nil {
		return "", fmt.Errorf("解析插件目录失败: %w", err)
	}
	root, err := filepath.Abs(a.resolveAppPath(filepath.ToSlash(filepath.Join("data", "extensions"))))
	if err != nil {
		return "", fmt.Errorf("解析插件根目录失败: %w", err)
	}
	target = filepath.Clean(target)
	root = filepath.Clean(root)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || rel == "" || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("拒绝删除插件根目录外的路径: %s", installDir)
	}
	return target, nil
}

func (a *App) removeBrowserExtensionFiles(extension BrowserExtension, installDir string) error {
	if installDir == "" {
		return nil
	}
	candidates := []string{installDir, installDir + ".tmp"}
	if matches, err := filepath.Glob(installDir + ".tmp-*"); err == nil {
		candidates = append(candidates, matches...)
	}

	manualDir, err := filepath.Abs(a.extensionManualDownloadDir())
	if err != nil {
		return err
	}
	if sourcePath := strings.TrimSpace(extension.SourceURL); filepath.IsAbs(sourcePath) && pathWithinDirectory(sourcePath, manualDir) {
		candidates = append(candidates, sourcePath)
	}
	if entries, readErr := os.ReadDir(manualDir); readErr == nil {
		for _, entry := range entries {
			name := strings.ToLower(strings.TrimSpace(entry.Name()))
			id := strings.ToLower(strings.TrimSpace(extension.ExtensionID))
			if name == id+".crx" || name == id+".zip" {
				candidates = append(candidates, filepath.Join(manualDir, entry.Name()))
			}
		}
	} else if !os.IsNotExist(readErr) {
		return fmt.Errorf("读取插件下载目录失败: %w", readErr)
	}

	for _, candidate := range appendUniquePaths(candidates) {
		if candidate == "" {
			continue
		}
		if candidate != installDir && !pathWithinDirectory(candidate, filepath.Dir(installDir)) && !pathWithinDirectory(candidate, manualDir) {
			return fmt.Errorf("拒绝清理插件目录外的路径: %s", candidate)
		}
		if err := os.RemoveAll(candidate); err != nil {
			return fmt.Errorf("删除插件文件失败 %s: %w", candidate, err)
		}
	}
	return nil
}

func pathWithinDirectory(path string, root string) bool {
	pathAbs, pathErr := filepath.Abs(path)
	rootAbs, rootErr := filepath.Abs(root)
	if pathErr != nil || rootErr != nil {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(rootAbs), filepath.Clean(pathAbs))
	return err == nil && rel != "." && rel != "" && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func appendUniquePaths(paths []string) []string {
	result := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		key := filepath.Clean(strings.TrimSpace(path))
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if key == "" || key == "." {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, path)
	}
	return result
}

func (a *App) BrowserProfileExtensionGet(profileID string) (BrowserProfileExtensionSettings, error) {
	if a.browserMgr == nil || a.browserMgr.ExtensionDAO == nil {
		return BrowserProfileExtensionSettings{}, fmt.Errorf("插件管理器未初始化")
	}
	settings, err := a.browserMgr.ExtensionDAO.GetProfileSettings(profileID)
	if err != nil {
		return BrowserProfileExtensionSettings{}, err
	}
	return a.populateAllowedProfileExtensions(settings), nil
}

func (a *App) BrowserProfileExtensionSave(profileID string, extensionIDs []string, configured bool) (BrowserProfileExtensionSettings, error) {
	if a.browserMgr == nil || a.browserMgr.ExtensionDAO == nil {
		return BrowserProfileExtensionSettings{}, fmt.Errorf("插件管理器未初始化")
	}
	settings, err := a.browserMgr.ExtensionDAO.SetProfileSettings(profileID, extensionIDs, configured)
	if err != nil {
		return BrowserProfileExtensionSettings{}, err
	}
	return a.populateAllowedProfileExtensions(settings), nil
}

func (a *App) populateAllowedProfileExtensions(settings BrowserProfileExtensionSettings) BrowserProfileExtensionSettings {
	items := a.browserMgr.AllowedExtensionsForProfile(settings.ProfileID)
	settings.AllowedExtensionIDs = make([]string, 0, len(items))
	for _, item := range items {
		settings.AllowedExtensionIDs = append(settings.AllowedExtensionIDs, item.ExtensionID)
	}
	return settings
}

func (a *App) BrowserExtensionProfileScopeGet(extensionID string) (BrowserExtensionProfileScope, error) {
	if a.browserMgr == nil || a.browserMgr.ExtensionDAO == nil {
		return BrowserExtensionProfileScope{}, fmt.Errorf("插件管理器未初始化")
	}
	extensionID = strings.TrimSpace(extensionID)
	if _, err := a.browserMgr.ExtensionDAO.Get(extensionID); err != nil {
		return BrowserExtensionProfileScope{}, err
	}
	return a.browserMgr.ExtensionDAO.GetExtensionProfileScope(extensionID)
}

func (a *App) BrowserExtensionProfileScopeSave(extensionID string, profileIDs []string, restricted bool) (BrowserExtensionProfileScope, error) {
	if a.browserMgr == nil || a.browserMgr.ExtensionDAO == nil {
		return BrowserExtensionProfileScope{}, fmt.Errorf("插件管理器未初始化")
	}
	extensionID = strings.TrimSpace(extensionID)
	if _, err := a.browserMgr.ExtensionDAO.Get(extensionID); err != nil {
		return BrowserExtensionProfileScope{}, err
	}
	known := make(map[string]struct{})
	deleted := make(map[string]struct{})
	for _, profile := range a.browserMgr.List() {
		known[profile.ProfileId] = struct{}{}
	}
	for _, profile := range a.browserMgr.ListDeleted() {
		known[profile.ProfileId] = struct{}{}
		deleted[profile.ProfileId] = struct{}{}
	}
	normalized := make([]string, 0, len(profileIDs))
	seen := make(map[string]struct{}, len(profileIDs))
	for _, profileID := range profileIDs {
		profileID = strings.TrimSpace(profileID)
		if profileID == "" {
			continue
		}
		if _, ok := known[profileID]; !ok {
			return BrowserExtensionProfileScope{}, fmt.Errorf("实例不存在: %s", profileID)
		}
		if _, ok := seen[profileID]; ok {
			continue
		}
		seen[profileID] = struct{}{}
		normalized = append(normalized, profileID)
	}
	// 回收站实例不显示在作用域弹窗中；编辑现有规则时保留它们原来的允许状态。
	if restricted {
		if current, err := a.browserMgr.ExtensionDAO.GetExtensionProfileScope(extensionID); err == nil && current.Restricted {
			for _, profileID := range current.ProfileIDs {
				if _, isDeleted := deleted[profileID]; !isDeleted {
					continue
				}
				if _, ok := seen[profileID]; ok {
					continue
				}
				seen[profileID] = struct{}{}
				normalized = append(normalized, profileID)
			}
		}
	}
	return a.browserMgr.ExtensionDAO.SetExtensionProfileScope(extensionID, normalized, restricted)
}
