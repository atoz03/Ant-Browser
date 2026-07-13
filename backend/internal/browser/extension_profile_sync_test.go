package browser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtensionMaintenanceBrowserIntegration(t *testing.T) {
	chrome := os.Getenv("ANT_BROWSER_EXTENSION_TEST_CHROME")
	extensionDir := os.Getenv("ANT_BROWSER_EXTENSION_TEST_DIR")
	if chrome == "" || extensionDir == "" {
		t.Skip("未配置 Chromium 插件集成测试路径")
	}

	userDataDir := t.TempDir()
	client, err := startExtensionMaintenanceBrowser(chrome, userDataDir, []string{extensionDir})
	if err != nil {
		t.Fatal(err)
	}
	extensionID, err := client.Load(extensionDir)
	if err != nil {
		client.Close()
		t.Fatal(err)
	}
	client.Close()
	if extensionID == "" {
		t.Fatal("Chromium 未返回插件 ID")
	}
	if !ProfileReferencesExtensionPath(userDataDir, extensionDir) {
		t.Fatal("插件没有持久化到 Chromium profile")
	}

	disabledClient, err := startExtensionMaintenanceBrowser(chrome, userDataDir, []string{})
	if err != nil {
		t.Fatal(err)
	}
	disabledItems, err := disabledClient.List()
	disabledClient.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range disabledItems {
		if item.ID == extensionID && item.Enabled {
			t.Fatal("空插件白名单没有停用已持久化插件")
		}
	}
	if !ProfileReferencesExtensionPath(userDataDir, extensionDir) {
		t.Fatal("停用插件不应卸载插件或删除 profile 记录")
	}
	reenabledClient, err := startExtensionMaintenanceBrowser(chrome, userDataDir, []string{extensionDir})
	if err != nil {
		t.Fatal(err)
	}
	reenabledItems, err := reenabledClient.List()
	reenabledClient.Close()
	if err != nil {
		t.Fatal(err)
	}
	reenabled := false
	for _, item := range reenabledItems {
		if item.ID == extensionID && item.Enabled {
			reenabled = true
			break
		}
	}
	if !reenabled {
		t.Fatal("恢复插件白名单后，已持久化插件没有重新启用")
	}

	if err := UninstallProfileExtension(chrome, userDataDir, extensionDir); err != nil {
		t.Fatal(err)
	}
	if ProfileReferencesExtensionPath(userDataDir, extensionDir) {
		t.Fatal("Chromium 标准卸载后仍残留插件路径")
	}
	for _, path := range []string{
		filepath.Join(userDataDir, "Default", "Local Extension Settings", extensionID),
		filepath.Join(userDataDir, "Default", "Sync Extension Settings", extensionID),
	} {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("Chromium 标准卸载后仍残留插件数据目录: %s", path)
		}
	}
	indexedDB, err := filepath.Glob(filepath.Join(userDataDir, "Default", "IndexedDB", "chrome-extension_"+extensionID+"_*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(indexedDB) > 0 {
		t.Fatalf("Chromium 标准卸载后仍残留插件 IndexedDB: %#v", indexedDB)
	}
}
