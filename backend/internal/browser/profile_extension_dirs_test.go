package browser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPersistedUnpackedExtensionDirsKeepsOnlyExternalDeveloperExtensions(t *testing.T) {
	root := t.TempDir()
	userDataDir := filepath.Join(root, "profile")
	managedRoot := filepath.Join(root, "managed")
	externalDir := filepath.Join(root, "developer-extension")
	managedDir := filepath.Join(managedRoot, "managed-extension")
	for _, dir := range []string{externalDir, managedDir, filepath.Join(userDataDir, "Default")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{externalDir, managedDir} {
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"name":"test","version":"1"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	prefs := `{"extensions":{"settings":{"external":{"location":4,"path":` + quoteJSON(externalDir) + `},"managed":{"location":4,"path":` + quoteJSON(managedDir) + `},"command":{"location":8,"path":` + quoteJSON(externalDir) + `}}}}`
	if err := os.WriteFile(filepath.Join(userDataDir, "Default", "Secure Preferences"), []byte(prefs), 0o644); err != nil {
		t.Fatal(err)
	}

	got := persistedUnpackedExtensionDirs(userDataDir, managedRoot)
	if len(got) != 1 || got[0] != externalDir {
		t.Fatalf("persistedUnpackedExtensionDirs() = %#v, want [%q]", got, externalDir)
	}
}

func quoteJSON(value string) string {
	data := make([]byte, 0, len(value)+2)
	data = append(data, '"')
	for _, char := range []byte(value) {
		if char == '\\' || char == '"' {
			data = append(data, '\\')
		}
		data = append(data, char)
	}
	data = append(data, '"')
	return string(data)
}
