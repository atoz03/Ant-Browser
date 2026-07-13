package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExtensionProfileSyncStateTracksVersionAndDisabledState(t *testing.T) {
	userDataDir := t.TempDir()
	managedRoot := filepath.Join(t.TempDir(), "extensions")
	extensionDir := filepath.Join(managedRoot, "extension-a")
	if err := os.MkdirAll(filepath.Join(userDataDir, "Default"), 0o755); err != nil {
		t.Fatal(err)
	}
	desired := []Extension{{ExtensionID: "extension-a", Version: "1.0.0", InstallDir: extensionDir}}
	if err := writeExtensionProfileSyncState(userDataDir, desired); err != nil {
		t.Fatal(err)
	}
	writeExtensionPreferences(t, userDataDir, extensionDir, nil)
	if !extensionProfileSyncStateMatches(userDataDir, managedRoot, desired) {
		t.Fatal("路径、版本和启用状态一致时应命中同步状态")
	}

	updated := []Extension{{ExtensionID: "extension-a", Version: "1.1.0", InstallDir: extensionDir}}
	if extensionProfileSyncStateMatches(userDataDir, managedRoot, updated) {
		t.Fatal("插件版本变化后必须重新同步")
	}

	writeExtensionPreferences(t, userDataDir, extensionDir, []int{1})
	if extensionProfileSyncStateMatches(userDataDir, managedRoot, desired) {
		t.Fatal("Chromium 内插件被停用后必须重新同步")
	}
}

func writeExtensionPreferences(t *testing.T, userDataDir string, extensionDir string, disableReasons []int) {
	t.Helper()
	payload := map[string]any{
		"extensions": map[string]any{
			"settings": map[string]any{
				"chromium-extension-id": map[string]any{
					"location":        4,
					"path":            extensionDir,
					"disable_reasons": disableReasons,
				},
			},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userDataDir, "Default", "Secure Preferences"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
