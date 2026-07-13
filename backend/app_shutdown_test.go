package backend

import "testing"

func TestPlatformSupportsCloseConfirmOnDesktopPlatforms(t *testing.T) {
	for _, goos := range []string{"windows", "darwin"} {
		if !platformSupportsTrayCloseFlowForOS(goos) {
			t.Fatalf("platformSupportsTrayCloseFlowForOS(%q) = false", goos)
		}
	}
	if platformSupportsTrayCloseFlowForOS("linux") {
		t.Fatal("Linux close flow should keep its existing direct-close behavior")
	}
}
