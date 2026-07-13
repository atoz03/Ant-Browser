//go:build !windows

package backend

import (
	"path/filepath"
	"testing"
)

func TestParseBrowserProcessesFromPSHandlesMacPathsWithSpaces(t *testing.T) {
	target := filepath.Join(t.TempDir(), "Ant Browser", "profile-1")
	output := "101 /Applications/Chromium.app/Contents/MacOS/Chromium --user-data-dir=" + target + " --remote-debugging-port=39222 --no-first-run\n" +
		"102 /Applications/Chromium.app/Contents/MacOS/Chromium --user-data-dir=/tmp/other --remote-debugging-port=39223\n"

	got := parseBrowserProcessesFromPS(output, target)
	if len(got) != 1 {
		t.Fatalf("processes = %#v, want one matching process", got)
	}
	if got[0].PID != 101 || got[0].DebugPort != 39222 {
		t.Fatalf("process = %#v", got[0])
	}
}

func TestCommandLineSwitchValueHandlesQuotedValue(t *testing.T) {
	got := commandLineSwitchValue(`Chromium --user-data-dir="/tmp/Profile One" --lang=en-US`, "--user-data-dir")
	if got != "/tmp/Profile One" {
		t.Fatalf("value = %q", got)
	}
}
