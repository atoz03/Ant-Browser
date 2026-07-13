package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/logger"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type browserStartInput struct {
	ProfileID            string
	ExtraLaunchArgs      []string
	StartURLs            []string
	SkipDefaultStartURLs bool
	PreferVisibleWindow  bool
	ForceDirectProxy     bool
	TemporaryProxyID     string
	TemporaryProxyConfig string
}

type browserStartPlan struct {
	profile              *BrowserProfile
	chromeBinaryPath     string
	userDataDir          string
	args                 []string
	extensionDirs        []string
	deferredStartTargets []string
	effectiveProxy       string
	acquiredProxyBridge  profileProxyBridgeRef
	releaseProxyBridge   bool
	assignedDebugPort    int
	startReadyTimeout    time.Duration
	startStableWindow    time.Duration
	maxStartAttempts     int
	totalReadyTimeout    time.Duration
}

var clearBrowserSessionRestoreData = browser.ClearSessionRestoreData

func newBrowserStartInput(profileID string, extraLaunchArgs []string, startURLs []string, skipDefaultStartURLs bool, preferVisibleWindow bool, forceDirectProxy bool, proxyID string, proxyConfig string) browserStartInput {
	normalizedExtraLaunchArgs := normalizeNonEmptyStrings(extraLaunchArgs)

	return browserStartInput{
		ProfileID:            profileID,
		ExtraLaunchArgs:      normalizedExtraLaunchArgs,
		StartURLs:            normalizeNonEmptyStrings(startURLs),
		SkipDefaultStartURLs: skipDefaultStartURLs,
		PreferVisibleWindow:  preferVisibleWindow,
		ForceDirectProxy:     forceDirectProxy,
		TemporaryProxyID:     strings.TrimSpace(proxyID),
		TemporaryProxyConfig: strings.TrimSpace(proxyConfig),
	}
}

func (input browserStartInput) hasTemporaryProxy() bool {
	return strings.TrimSpace(input.TemporaryProxyID) != "" || strings.TrimSpace(input.TemporaryProxyConfig) != ""
}

func (plan *browserStartPlan) releaseBridgeIfNeeded(a *App) {
	if plan == nil || a == nil {
		return
	}
	if plan.releaseProxyBridge {
		a.releaseProxyBridgeRef(plan.acquiredProxyBridge)
	}
}

func (a *App) resolveBrowserStartProfile(input browserStartInput) (*BrowserProfile, bool, error) {
	log := logger.New("Browser")

	profile, exists := a.browserMgr.Profiles[input.ProfileID]
	if !exists {
		err := fmt.Errorf("实例启动失败：未找到实例配置（ID=%s）。请刷新列表后重试。", input.ProfileID)
		log.Error("实例不存在", logger.F("profile_id", input.ProfileID), logger.F("reason", err.Error()))
		return nil, false, err
	}
	a.ensureProfileLaunchCode(profile)

	if !profile.Running {
		return profile, false, nil
	}

	if !isBrowserProfileLive(profile, a.browserMgr.BrowserProcesses[input.ProfileID]) {
		log.Info("检测到实例运行状态已失效，准备重新启动",
			logger.F("profile_id", input.ProfileID),
			logger.F("pid", profile.Pid),
			logger.F("debug_port", profile.DebugPort),
		)
		a.markProfileStoppedLocked(input.ProfileID, profile)
		return profile, false, nil
	}

	if len(normalizeNonEmptyStrings(input.StartURLs)) == 0 && len(normalizeNonEmptyStrings(input.ExtraLaunchArgs)) == 0 {
		if a.launchServer != nil && profile.DebugReady {
			a.launchServer.SetActiveProfile(profile)
		}
		a.emitBrowserInstanceStarted(profile, true)
		return profile, true, nil
	}

	if err := a.openBrowserTabForRunningProfile(profile, input.ExtraLaunchArgs, input.StartURLs); err != nil {
		startErr := fmt.Errorf("实例已在运行，但新标签打开失败：%w", err)
		log.Error("运行中实例新标签打开失败",
			logger.F("profile_id", input.ProfileID),
			logger.F("debug_port", profile.DebugPort),
			logger.F("error", err.Error()),
			logger.F("reason", startErr.Error()),
		)
		profile.LastError = startErr.Error()
		return profile, true, startErr
	}

	if a.launchServer != nil && profile.DebugReady {
		a.launchServer.SetActiveProfile(profile)
	}
	a.emitBrowserInstanceStarted(profile, true)
	return profile, true, nil
}

func (a *App) prepareBrowserStartPlan(input browserStartInput, profile *BrowserProfile) (*browserStartPlan, error) {
	bookmarks := a.BookmarkList()
	sanitizedProfileLaunchArgs, sanitizedExtraLaunchArgs, chromeBinaryPath, userDataDir, err := a.prepareBrowserLaunchContext(input, profile, bookmarks)
	if err != nil {
		return nil, err
	}
	if err := a.browserMgr.SyncProfileExtensions(input.ProfileID, chromeBinaryPath, userDataDir); err != nil {
		startErr := fmt.Errorf("实例启动失败：插件同步失败。原因：%w", err)
		logger.New("Browser").Error("实例插件同步失败",
			logger.F("profile_id", input.ProfileID),
			logger.F("user_data_dir", userDataDir),
			logger.F("error", err.Error()),
		)
		profile.LastError = startErr.Error()
		return nil, startErr
	}

	effectiveProxy, acquiredProxyBridge, releaseProxyBridge, err := a.resolveBrowserStartProxy(input, profile)
	if err != nil {
		return nil, err
	}

	startReadyTimeout, startStableWindow := a.browserStartTimingSettings()
	maxStartAttempts := browserStartAttemptCount()
	totalReadyTimeout := time.Duration(maxStartAttempts) * startReadyTimeout
	restoreLastSession := browserRestoreLastSession(a.config)
	extensionDirs := a.browserMgr.LaunchExtensionDirsForProfile(input.ProfileID, userDataDir)
	enforceExtensionAllowList := a.browserMgr.HasManagedExtensions() || len(extensionDirs) > 0
	defaultStartURLs := mergeStartURLs(browserDefaultStartURLs(a.config), bookmarkStartURLs(bookmarks))
	launchTargets, deferredStartTargets := buildBrowserLaunchTargets(
		input.StartURLs,
		defaultStartURLs,
		input.SkipDefaultStartURLs,
		restoreLastSession,
		browserLightStartEnabled(a.config),
	)
	// macOS 的 ICU locale 只能通过 CDP 在导航前覆盖。即使关闭了轻启动，
	// 也要先停在 about:blank，待接管完成后再创建真实启动页。
	if requiresLocaleOverrideBeforeNavigation(profile.FingerprintArgs) && len(deferredStartTargets) == 0 {
		configuredTargets := resolveConfiguredStartTargets(input.StartURLs, defaultStartURLs, input.SkipDefaultStartURLs)
		if len(configuredTargets) > 0 {
			launchTargets = []string{"about:blank"}
			deferredStartTargets = configuredTargets
		}
	}

	assignedDebugPort, err := nextAvailablePort()
	if err != nil {
		startErr := fmt.Errorf("实例启动失败：本地调试端口分配失败。原因：%v。请关闭占用端口的程序后重试。", err)
		logger.New("Browser").Error("调试端口分配失败",
			logger.F("profile_id", input.ProfileID),
			logger.F("error", err.Error()),
			logger.F("reason", startErr.Error()),
		)
		profile.LastError = startErr.Error()
		return nil, startErr
	}

	return &browserStartPlan{
		profile:              profile,
		chromeBinaryPath:     chromeBinaryPath,
		userDataDir:          userDataDir,
		extensionDirs:        extensionDirs,
		args:                 buildBrowserLaunchArgs(profile, userDataDir, assignedDebugPort, effectiveProxy, extensionDirs, enforceExtensionAllowList, sanitizedProfileLaunchArgs, sanitizedExtraLaunchArgs, launchTargets),
		deferredStartTargets: deferredStartTargets,
		effectiveProxy:       effectiveProxy,
		acquiredProxyBridge:  acquiredProxyBridge,
		releaseProxyBridge:   releaseProxyBridge,
		assignedDebugPort:    assignedDebugPort,
		startReadyTimeout:    startReadyTimeout,
		startStableWindow:    startStableWindow,
		maxStartAttempts:     maxStartAttempts,
		totalReadyTimeout:    totalReadyTimeout,
	}, nil
}

func (a *App) prepareBrowserLaunchContext(input browserStartInput, profile *BrowserProfile, bookmarks []BrowserBookmark) ([]string, []string, string, string, error) {
	log := logger.New("Browser")

	sanitizedProfileLaunchArgs, managedProfileArgs := sanitizeManagedLaunchArgs(profile.LaunchArgs)
	sanitizedExtraLaunchArgs, managedExtraArgs := sanitizeManagedLaunchArgs(input.ExtraLaunchArgs)
	logManagedLaunchArgOverrides(log, input.ProfileID, "profile.launchArgs", managedProfileArgs)
	logManagedLaunchArgOverrides(log, input.ProfileID, "start.extraLaunchArgs", managedExtraArgs)

	proxyChanged := a.browserMgr.ApplyDefaults(profile)
	if proxyChanged {
		_ = a.browserMgr.SaveProfiles()
	}

	chromeBinaryPath, err := a.browserMgr.ResolveChromeBinary(profile)
	if err != nil {
		startErr := fmt.Errorf("实例启动失败：%w", err)
		log.Error("内核路径解析失败",
			logger.F("profile_id", input.ProfileID),
			logger.F("error", err.Error()),
			logger.F("reason", startErr.Error()),
		)
		profile.LastError = startErr.Error()
		return nil, nil, "", "", startErr
	}

	userDataDir := a.browserMgr.ResolveUserDataDir(profile)
	if err := os.MkdirAll(userDataDir, 0o755); err != nil {
		startErr := fmt.Errorf("实例启动失败：无法创建用户数据目录 %s。原因：%w。请检查目录权限或路径配置。", userDataDir, err)
		log.Error("用户数据目录创建失败",
			logger.F("profile_id", input.ProfileID),
			logger.F("dir", userDataDir),
			logger.F("error", err.Error()),
			logger.F("reason", startErr.Error()),
		)
		profile.LastError = startErr.Error()
		return nil, nil, "", "", startErr
	}

	if err := browser.EnsureDefaultBookmarks(userDataDir, bookmarks); err != nil {
		log.Error("默认书签写入失败", logger.F("error", err.Error()))
	}

	if detection, ok := detectBrowserRuntimeByActivePort(userDataDir); ok && detection.DebugReady {
		a.markProfileRunningLocked(input.ProfileID, profile, nil, detection.PID, detection.DebugPort, true, "")
		log.Warn("检测到同一用户数据目录已有浏览器运行，已接管为当前实例状态",
			logger.F("profile_id", input.ProfileID),
			logger.F("user_data_dir", userDataDir),
			logger.F("pid", detection.PID),
			logger.F("debug_port", detection.DebugPort),
		)
		if len(normalizeNonEmptyStrings(input.StartURLs)) == 0 && len(normalizeNonEmptyStrings(input.ExtraLaunchArgs)) == 0 {
			return nil, nil, "", "", errBrowserStartHandledByRecoveredRuntime
		}
		if err := a.openBrowserTabForRunningProfile(profile, input.ExtraLaunchArgs, input.StartURLs); err != nil {
			startErr := fmt.Errorf("实例已在运行，但新标签打开失败：%w", err)
			profile.LastError = startErr.Error()
			return nil, nil, "", "", startErr
		}
		return nil, nil, "", "", errBrowserStartHandledByRecoveredRuntime
	}

	if !browserRestoreLastSession(a.config) {
		if err := clearBrowserSessionRestoreData(userDataDir); err != nil {
			if terminated, terminateErr := terminateBrowserProcessesByUserDataDir(userDataDir, 5*time.Second); terminateErr == nil && terminated {
				log.Warn("会话缓存被旧浏览器进程占用，已结束占用进程并重试清理",
					logger.F("profile_id", input.ProfileID),
					logger.F("user_data_dir", userDataDir),
				)
				if retryErr := clearBrowserSessionRestoreData(userDataDir); retryErr == nil {
					return sanitizedProfileLaunchArgs, sanitizedExtraLaunchArgs, chromeBinaryPath, userDataDir, nil
				} else {
					err = retryErr
				}
			} else if terminateErr != nil {
				log.Warn("会话缓存清理失败后尝试结束占用进程失败",
					logger.F("profile_id", input.ProfileID),
					logger.F("user_data_dir", userDataDir),
					logger.F("error", terminateErr.Error()),
				)
			}
			sessionDir := filepath.Join(userDataDir, "Default", "Sessions")
			startErr := fmt.Errorf("实例启动失败：无法清理上次会话缓存 %s。原因：%w。请关闭占用该目录的浏览器进程后重试。", sessionDir, err)
			log.Error("会话恢复缓存清理失败",
				logger.F("profile_id", input.ProfileID),
				logger.F("dir", sessionDir),
				logger.F("error", err.Error()),
				logger.F("reason", startErr.Error()),
			)
			profile.LastError = startErr.Error()
			return nil, nil, "", "", startErr
		}
	}

	return sanitizedProfileLaunchArgs, sanitizedExtraLaunchArgs, chromeBinaryPath, userDataDir, nil
}

func buildBrowserLaunchArgs(profile *BrowserProfile, userDataDir string, debugPort int, effectiveProxy string, extensionDirs []string, enforceExtensionAllowList bool, sanitizedProfileLaunchArgs []string, sanitizedExtraLaunchArgs []string, launchTargets []string) []string {
	args := []string{
		fmt.Sprintf("--user-data-dir=%s", userDataDir),
		fmt.Sprintf("--remote-debugging-port=%d", debugPort),
		"--disable-session-crashed-bubble",
	}

	fingerprintArgs := normalizeFingerprintLaunchArgs(profile.FingerprintArgs)
	hasFingerprint := false
	for _, arg := range fingerprintArgs {
		if strings.HasPrefix(arg, "--fingerprint=") {
			hasFingerprint = true
			break
		}
	}
	if !hasFingerprint {
		args = append(args, fmt.Sprintf("--fingerprint=%d", deterministicFingerprintSeed(profile.ProfileId)))
	}

	if effectiveProxy == "direct://" {
		args = append(args, "--no-proxy-server")
	} else if effectiveProxy != "" {
		args = append(args, fmt.Sprintf("--proxy-server=%s", effectiveProxy))
	}

	if extensionArg := strings.Join(normalizeNonEmptyStrings(extensionDirs), ","); extensionArg != "" || enforceExtensionAllowList {
		args = append(args, fmt.Sprintf("--disable-extensions-except=%s", extensionArg))
	}

	args = append(args, fingerprintArgs...)
	args = append(args, sanitizedProfileLaunchArgs...)
	args = append(args, sanitizedExtraLaunchArgs...)
	return browser.BuildLaunchArgs(args, launchTargets)
}
