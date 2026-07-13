//go:build !darwin

package browser

import "fmt"

func extractDMGArchive(archivePath, dest string, progressCb func(int, string)) error {
	return fmt.Errorf("DMG 内核包只能在 macOS 上导入")
}
