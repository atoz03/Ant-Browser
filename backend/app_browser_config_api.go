package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/config"
	"ant-chrome/backend/internal/logger"
	"fmt"
	"path/filepath"
	"strings"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) GetBrowserSettings() BrowserSettings {
	return BrowserSettings{
		UserDataRoot:           a.config.Browser.UserDataRoot,
		DefaultFingerprintArgs: append([]string{}, a.config.Browser.DefaultFingerprintArgs...),
		DefaultLaunchArgs:      append([]string{}, a.config.Browser.DefaultLaunchArgs...),
		DefaultStartURLs:       append([]string{}, a.config.Browser.DefaultStartURLs...),
		LightStartEnabled:      browserLightStartEnabled(a.config),
		RestoreLastSession:     a.config.Browser.RestoreLastSession,
		StartReadyTimeoutMs:    browserStartReadyTimeoutMillis(a.config),
		StartStableWindowMs:    browserStartStableWindowMillis(a.config),
		DefaultConnectorType:   config.NormalizeBrowserConnectorType(a.config.Browser.DefaultConnectorType),
	}
}

func (a *App) SaveBrowserSettings(settings BrowserSettings) error {
	log := logger.New("Browser")
	a.config.Browser.UserDataRoot = strings.TrimSpace(settings.UserDataRoot)
	a.config.Browser.DefaultFingerprintArgs = append([]string{}, settings.DefaultFingerprintArgs...)
	a.config.Browser.DefaultLaunchArgs = append([]string{}, settings.DefaultLaunchArgs...)
	if settings.DefaultStartURLs != nil {
		a.config.Browser.DefaultStartURLs = normalizeNonEmptyStrings(settings.DefaultStartURLs)
	} else if a.config.Browser.DefaultStartURLs == nil {
		a.config.Browser.DefaultStartURLs = config.DefaultBrowserStartURLs()
	}
	lightStartEnabled := settings.LightStartEnabled
	a.config.Browser.LightStartEnabled = &lightStartEnabled
	a.config.Browser.RestoreLastSession = settings.RestoreLastSession
	a.config.Browser.DefaultConnectorType = config.NormalizeBrowserConnectorType(settings.DefaultConnectorType)
	if settings.StartReadyTimeoutMs > 0 {
		a.config.Browser.StartReadyTimeoutMs = settings.StartReadyTimeoutMs
	} else if a.config.Browser.StartReadyTimeoutMs <= 0 {
		a.config.Browser.StartReadyTimeoutMs = browserStartReadyTimeoutMillis(nil)
	}
	if settings.StartStableWindowMs > 0 {
		a.config.Browser.StartStableWindowMs = settings.StartStableWindowMs
	} else if a.config.Browser.StartStableWindowMs <= 0 {
		a.config.Browser.StartStableWindowMs = browserStartStableWindowMillis(nil)
	}
	if err := a.config.Save(a.resolveAppPath("config.yaml")); err != nil {
		log.Error("浏览器配置保存失败", logger.F("error", err))
		return err
	}
	return nil
}

func (a *App) BrowserCoreList() []BrowserCore {
	return a.browserMgr.ListCores()
}

func (a *App) BrowserCoreSave(input BrowserCoreInput) error {
	return a.browserMgr.SaveCore(input)
}

func (a *App) BrowserCoreDelete(coreId string) error {
	return a.browserMgr.DeleteCore(coreId)
}

func (a *App) BrowserCoreSetDefault(coreId string) error {
	return a.browserMgr.SetDefaultCore(coreId)
}

func (a *App) BrowserCoreValidate(corePath string) BrowserCoreValidateResult {
	return a.browserMgr.ValidateCorePath(corePath)
}

func (a *App) BrowserCoreExtendedInfo() []BrowserCoreExtendedInfo {
	return a.browserMgr.GetCoresExtendedInfo()
}

// BrowserCoreScan 重新扫描 chrome 目录，自动注册新内核
func (a *App) BrowserCoreScan() []BrowserCore {
	a.autoDetectCores()
	return a.browserMgr.ListCores()
}

// BrowserCoreImportLocal 选择一个已解压内核目录并直接注册，不下载、不复制文件。
func (a *App) BrowserCoreImportLocal() (*BrowserCore, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("app context is nil")
	}
	if a.browserMgr == nil {
		return nil, fmt.Errorf("browser manager is nil")
	}

	selectedDir, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "选择已解压的 Chrome 内核目录",
	})
	if err != nil {
		return nil, err
	}
	selectedDir = strings.TrimSpace(selectedDir)
	if selectedDir == "" {
		return nil, nil
	}

	absDir, err := filepath.Abs(selectedDir)
	if err != nil {
		return nil, err
	}
	if _, _, ok := browser.FindCoreExecutable(absDir); !ok {
		return nil, fmt.Errorf("所选目录不是有效内核目录：未找到浏览器可执行文件（候选：%s）", strings.Join(browser.CoreExecutableCandidates(), ", "))
	}

	corePath := a.relativeCorePathIfPossible(absDir)
	coreName := strings.TrimSpace(filepath.Base(absDir))
	if coreName == "" || coreName == "." || coreName == string(filepath.Separator) {
		coreName = "本地内核"
	}

	for _, existing := range a.browserMgr.ListCores() {
		if normalizeCorePathForCompare(existing.CorePath) == normalizeCorePathForCompare(corePath) {
			return &existing, nil
		}
	}

	input := browser.CoreInput{
		CoreName:  coreName,
		CorePath:  corePath,
		IsDefault: len(a.browserMgr.ListCores()) == 0,
	}
	if err := a.browserMgr.SaveCore(input); err != nil {
		return nil, err
	}

	for _, saved := range a.browserMgr.ListCores() {
		if normalizeCorePathForCompare(saved.CorePath) == normalizeCorePathForCompare(corePath) {
			return &saved, nil
		}
	}
	return nil, fmt.Errorf("本地内核已保存但未能读取结果")
}

func (a *App) relativeCorePathIfPossible(absDir string) string {
	for _, root := range []string{a.appRootAbs(), a.appStateRootAbs()} {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(rootAbs, absDir)
		if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(filepath.ToSlash(rel), "../") && !filepath.IsAbs(rel) {
			return filepath.ToSlash(rel)
		}
	}
	return absDir
}

// BrowserCoreDownload 在线下载并自动解压配置内核
func (a *App) BrowserCoreDownload(coreName, url, proxyConfig string) error {
	if a.ctx == nil {
		return fmt.Errorf("app context is nil")
	}
	go a.browserMgr.DownloadAndExtractCore(a.ctx, coreName, url, proxyConfig)
	return nil
}

// BrowserCoreRedownload 重新下载并替换指定内核目录
func (a *App) BrowserCoreRedownload(coreId, url, proxyConfig string) error {
	if a.ctx == nil {
		return fmt.Errorf("app context is nil")
	}
	go a.browserMgr.RedownloadCore(a.ctx, coreId, url, proxyConfig)
	return nil
}
