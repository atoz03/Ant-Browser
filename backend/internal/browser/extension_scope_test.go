package browser

import (
	"ant-chrome/backend/internal/config"
	"ant-chrome/backend/internal/database"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func newExtensionScopeTestManager(t *testing.T) (*Manager, *SQLiteExtensionDAO) {
	t.Helper()
	root := t.TempDir()
	db, err := database.NewDB(filepath.Join(root, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	dao := NewSQLiteExtensionDAO(db.GetConn())
	manager := NewManager(config.DefaultConfig(), root)
	manager.ExtensionDAO = dao
	return manager, dao
}

func TestEnabledExtensionsForProfileIntersectsGlobalScopeAndProfileConfig(t *testing.T) {
	manager, dao := newExtensionScopeTestManager(t)
	for _, extension := range []Extension{
		{ExtensionID: "extension-a", Name: "A", InstallDir: "/old/a", Enabled: true},
		{ExtensionID: "extension-b", Name: "B", InstallDir: "/old/b", Enabled: true},
		{ExtensionID: "extension-c", Name: "C", InstallDir: "/old/c", Enabled: false},
	} {
		if err := dao.Upsert(extension); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := dao.SetExtensionProfileScope("extension-b", []string{"profile-1"}, true); err != nil {
		t.Fatal(err)
	}

	assertExtensionIDs(t, manager.EnabledExtensionsForProfile("profile-1"), "extension-a", "extension-b")
	assertExtensionIDs(t, manager.EnabledExtensionsForProfile("profile-2"), "extension-a")

	if _, err := dao.SetProfileSettings("profile-1", []string{"extension-a"}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := dao.SetProfileSettings("profile-2", []string{"extension-a", "extension-b", "extension-c"}, true); err != nil {
		t.Fatal(err)
	}
	assertExtensionIDs(t, manager.EnabledExtensionsForProfile("profile-1"), "extension-a")
	assertExtensionIDs(t, manager.EnabledExtensionsForProfile("profile-2"), "extension-a")
}

func TestExtensionScopeDoesNotConvertProfileToStaticConfiguration(t *testing.T) {
	_, dao := newExtensionScopeTestManager(t)
	if err := dao.Upsert(Extension{ExtensionID: "extension-a", Name: "A", InstallDir: "/old/a", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := dao.SetExtensionProfileScope("extension-a", []string{"profile-1"}, true); err != nil {
		t.Fatal(err)
	}
	settings, err := dao.GetProfileSettings("profile-2")
	if err != nil {
		t.Fatal(err)
	}
	if settings.Configured || len(settings.ExtensionIDs) != 0 {
		t.Fatalf("插件作用域不应改写单实例白名单: %#v", settings)
	}
}

func TestManagedExtensionInstallDirUsesCurrentDeviceRoot(t *testing.T) {
	manager, dao := newExtensionScopeTestManager(t)
	extensionID := "extension-a"
	currentDir := manager.ManagedExtensionInstallDir(extensionID)
	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(currentDir, "manifest.json"), []byte(`{"name":"A","version":"1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := dao.Upsert(Extension{ExtensionID: extensionID, Name: "A", InstallDir: `C:\\old-device\\extensions\\extension-a`, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	items, err := manager.ListExtensions()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].InstallDir != currentDir {
		t.Fatalf("应使用当前设备插件目录，得到 %#v", items)
	}
}

func TestExtensionPathComparisonRespectsCaseSensitivePlatforms(t *testing.T) {
	if runtime.GOOS == "windows" {
		if !sameExtensionPath(`C:\\Extensions\\Demo`, `c:\\extensions\\demo`) {
			t.Fatal("Windows 路径比较应忽略大小写")
		}
		return
	}
	if sameExtensionPath("/tmp/Extensions/Demo", "/tmp/extensions/demo") {
		t.Fatal("大小写敏感平台不应合并不同路径")
	}
}

func assertExtensionIDs(t *testing.T, items []Extension, expected ...string) {
	t.Helper()
	if len(items) != len(expected) {
		t.Fatalf("插件数量不符: got=%#v want=%#v", extensionIDs(items), expected)
	}
	for index, item := range items {
		if item.ExtensionID != expected[index] {
			t.Fatalf("插件列表不符: got=%#v want=%#v", extensionIDs(items), expected)
		}
	}
}

func extensionIDs(items []Extension) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ExtensionID)
	}
	return ids
}
