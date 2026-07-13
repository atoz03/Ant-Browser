//go:build !darwin

package browser

func readPlatformCoreVersion(baseDir string) string {
	return ""
}
