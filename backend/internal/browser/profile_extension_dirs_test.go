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

func TestProfileReferencesExtensionPathChecksBothPreferenceFiles(t *testing.T) {
	root := t.TempDir()
	userDataDir := filepath.Join(root, "profile")
	installDir := filepath.Join(root, "managed", "extension")
	if err := os.MkdirAll(filepath.Join(userDataDir, "Default"), 0o755); err != nil {
		t.Fatal(err)
	}
	prefs := `{"extensions":{"settings":{"test":{"location":8,"path":` + quoteJSON(installDir) + `}}}}`
	if err := os.WriteFile(filepath.Join(userDataDir, "Default", "Preferences"), []byte(prefs), 0o644); err != nil {
		t.Fatal(err)
	}
	if !ProfileReferencesExtensionPath(userDataDir, installDir) {
		t.Fatal("Preferences 中的旧命令行插件记录应被识别")
	}
	if ProfileReferencesExtensionPath(userDataDir, filepath.Join(root, "other")) {
		t.Fatal("不应匹配其他插件目录")
	}
}

func TestPersistedManagedUnpackedExtensionDirsIgnoresOldCommandLineRecords(t *testing.T) {
	root := t.TempDir()
	userDataDir := filepath.Join(root, "profile")
	managedRoot := filepath.Join(root, "managed")
	persistedDir := filepath.Join(managedRoot, "persisted")
	commandLineDir := filepath.Join(managedRoot, "command-line")
	if err := os.MkdirAll(filepath.Join(userDataDir, "Default"), 0o755); err != nil {
		t.Fatal(err)
	}
	prefs := `{"extensions":{"settings":{"persisted":{"location":4,"path":` + quoteJSON(persistedDir) + `},"old":{"location":8,"path":` + quoteJSON(commandLineDir) + `}}}}`
	if err := os.WriteFile(filepath.Join(userDataDir, "Default", "Secure Preferences"), []byte(prefs), 0o644); err != nil {
		t.Fatal(err)
	}
	got := persistedManagedUnpackedExtensions(userDataDir, managedRoot)
	item, ok := got[normalizedExtensionPath(persistedDir)]
	if len(got) != 1 || !ok || item.Path != persistedDir {
		t.Fatalf("persistedManagedUnpackedExtensions() = %#v, want [%q]", got, persistedDir)
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
