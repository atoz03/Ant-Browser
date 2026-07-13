package backend

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveBrowserExtensionFilesCleansManagedAndTemporaryFiles(t *testing.T) {
	root := t.TempDir()
	app := NewApp(root)
	extensionID := "abcdefghijklmnopabcdefghijklmnop"
	installDir := filepath.Join(root, "data", "extensions", extensionID)
	manualDir := filepath.Join(root, "data", "extensions", "manual-downloads")
	pathsToRemove := []string{
		installDir,
		installDir + ".tmp",
		installDir + ".tmp-12345",
		filepath.Join(manualDir, extensionID+".crx"),
		filepath.Join(manualDir, "imported.zip"),
	}
	for _, path := range pathsToRemove {
		if filepath.Ext(path) == "" || filepath.Base(path) == extensionID || filepath.Base(path) == extensionID+".tmp" || filepath.Base(path) == extensionID+".tmp-12345" {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	keepPath := filepath.Join(manualDir, "keep.crx")
	if err := os.WriteFile(keepPath, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	extension := BrowserExtension{
		ExtensionID: extensionID,
		InstallDir:  installDir,
		SourceURL:   filepath.Join(manualDir, "imported.zip"),
	}
	if err := app.removeBrowserExtensionFiles(extension, installDir); err != nil {
		t.Fatal(err)
	}
	for _, path := range pathsToRemove {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("插件残留未删除: %s", path)
		}
	}
	if _, err := os.Stat(keepPath); err != nil {
		t.Fatalf("不相关的下载文件不应被删除: %v", err)
	}
}
