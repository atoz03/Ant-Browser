//go:build darwin

package browser

import (
	"os/exec"
	"path/filepath"
	"strings"
)

func readPlatformCoreVersion(baseDir string) string {
	executable, _, ok := FindCoreExecutable(baseDir)
	if !ok {
		return ""
	}
	path := filepath.Clean(executable)
	for path != filepath.Dir(path) {
		if strings.HasSuffix(strings.ToLower(path), ".app") {
			plistPath := filepath.Join(path, "Contents", "Info.plist")
			output, err := exec.Command("plutil", "-extract", "CFBundleShortVersionString", "raw", "-o", "-", plistPath).Output()
			if err == nil {
				return strings.TrimSpace(string(output))
			}
			return ""
		}
		path = filepath.Dir(path)
	}
	return ""
}
