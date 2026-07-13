package backend

import (
	"strings"
	"testing"
)

func TestBuildBrowserLaunchArgsDoesNotReloadPersistedExtensions(t *testing.T) {
	args := buildBrowserLaunchArgs(
		&BrowserProfile{ProfileId: "profile-1"},
		"/tmp/profile-1",
		39222,
		"direct://",
		[]string{"/tmp/extensions/example"},
		true,
		nil,
		nil,
		[]string{"about:blank"},
	)
	hasAllowList := false
	hasLoadExtension := false
	for _, arg := range args {
		hasAllowList = hasAllowList || strings.HasPrefix(arg, "--disable-extensions-except=")
		hasLoadExtension = hasLoadExtension || strings.HasPrefix(arg, "--load-extension=")
	}
	if !hasAllowList {
		t.Fatal("插件目录应保留在 Chromium 白名单中")
	}
	if hasLoadExtension {
		t.Fatal("已持久化插件不应在每次启动时再次通过 --load-extension 加载")
	}
}

func TestBuildBrowserLaunchArgsDisablesPersistedExtensionsWhenAllowListIsEmpty(t *testing.T) {
	args := buildBrowserLaunchArgs(
		&BrowserProfile{ProfileId: "profile-1"},
		"/tmp/profile-1",
		39222,
		"direct://",
		nil,
		true,
		nil,
		nil,
		[]string{"about:blank"},
	)
	for _, arg := range args {
		if arg == "--disable-extensions-except=" {
			return
		}
	}
	t.Fatal("插件全部停用时仍应传入空白名单，防止已持久化插件重新启用")
}
