package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/config"
	"net/http"
	"os/exec"
	"reflect"
	"testing"
	"time"
)

func TestWaitForBrowserDebugReadyMarksProfileReady(t *testing.T) {
	t.Parallel()

	port := freeLoopbackPort(t)
	app := NewApp("")
	app.browserMgr = browser.NewManager(config.DefaultConfig(), "")
	app.browserMgr.Profiles = map[string]*BrowserProfile{
		"profile-ready": {
			ProfileId:      "profile-ready",
			ProfileName:    "Ready Browser",
			Running:        true,
			DebugPort:      port,
			DebugReady:     false,
			RuntimeWarning: "pending",
			LastStartAt:    time.Now().Format(time.RFC3339),
		},
	}
	app.browserMgr.BrowserProcesses = make(map[string]*exec.Cmd)

	serverReady := make(chan *devToolsTestServer, 1)
	go func() {
		time.Sleep(200 * time.Millisecond)
		serverReady <- startDevToolsServerOnPort(t, port, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/json/version":
				_, _ = w.Write([]byte(`{"Browser":"Chrome/142.0","webSocketDebuggerUrl":"ws://127.0.0.1/devtools/browser"}`))
			case "/json/list":
				_, _ = w.Write([]byte(`[{"id":"page-1"}]`))
			default:
				http.NotFound(w, r)
			}
		}))
	}()

	snapshot, changed := app.waitForBrowserDebugReady("profile-ready", port, 2*time.Second)
	server := <-serverReady
	defer server.Close()

	if snapshot == nil {
		t.Fatal("期望等待到调试接口就绪")
	}
	if !changed {
		t.Fatal("期望调试接口就绪后标记实例状态变更")
	}
	if !snapshot.DebugReady {
		t.Fatal("期望实例被标记为调试接口已就绪")
	}
	if snapshot.RuntimeWarning != "" {
		t.Fatalf("期望调试接口就绪后清空警告，实际=%q", snapshot.RuntimeWarning)
	}
}

func TestSanitizeManagedLaunchArgsRemovesSystemManagedFlags(t *testing.T) {
	t.Parallel()

	got, removed := sanitizeManagedLaunchArgs([]string{
		"--lang=en-US",
		"--remote-debugging-port=9222",
		"--user-data-dir", "D:\\profiles\\demo",
		"--proxy-server", "http://127.0.0.1:9000",
		"--remote-debugging-pipe",
		"https://example.com",
	})

	wantArgs := []string{"--lang=en-US", "https://example.com"}
	if !reflect.DeepEqual(got, wantArgs) {
		t.Fatalf("sanitizeManagedLaunchArgs args mismatch: got=%v want=%v", got, wantArgs)
	}

	wantRemoved := []string{
		"--remote-debugging-port",
		"--user-data-dir",
		"--proxy-server",
		"--remote-debugging-pipe",
	}
	if !reflect.DeepEqual(removed, wantRemoved) {
		t.Fatalf("sanitizeManagedLaunchArgs removed mismatch: got=%v want=%v", removed, wantRemoved)
	}
}

func TestSanitizeManagedLaunchArgsKeepsUnmanagedFlags(t *testing.T) {
	t.Parallel()

	input := []string{"--lang=en-US", "--disable-sync", "https://example.com"}
	got, removed := sanitizeManagedLaunchArgs(input)
	if !reflect.DeepEqual(got, input) {
		t.Fatalf("sanitizeManagedLaunchArgs should preserve unmanaged args: got=%v want=%v", got, input)
	}
	if len(removed) != 0 {
		t.Fatalf("sanitizeManagedLaunchArgs should not report managed args, got=%v", removed)
	}
}

func TestResolveBrowserStartProxyUsesTemporaryProxyWithoutMutatingProfile(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()
	cfg.Browser.Proxies = []config.BrowserProxy{
		{ProxyId: "stored-proxy", ProxyName: "Stored", ProxyConfig: "http://127.0.0.1:18080"},
		{ProxyId: "runtime-proxy", ProxyName: "Runtime", ProxyConfig: "http://127.0.0.1:28080"},
	}
	app := NewApp("")
	app.config = cfg
	app.browserMgr = browser.NewManager(cfg, t.TempDir())
	profile := &BrowserProfile{
		ProfileId:   "profile-temporary-proxy",
		ProfileName: "Temporary Proxy",
		ProxyId:     "stored-proxy",
		ProxyConfig: "http://127.0.0.1:18080",
	}
	input := newBrowserStartInput(profile.ProfileId, nil, nil, false, false, false, "runtime-proxy", "")

	effectiveProxy, bridgeRef, releaseBridge, err := app.resolveBrowserStartProxy(input, profile)
	if err != nil {
		t.Fatalf("resolveBrowserStartProxy returned error: %v", err)
	}
	if effectiveProxy != "http://127.0.0.1:28080" {
		t.Fatalf("expected temporary proxy, got %q", effectiveProxy)
	}
	if bridgeRef.valid() || releaseBridge {
		t.Fatalf("plain HTTP proxy should not acquire bridge: ref=%+v release=%v", bridgeRef, releaseBridge)
	}
	if profile.ProxyId != "stored-proxy" || profile.ProxyConfig != "http://127.0.0.1:18080" {
		t.Fatalf("temporary proxy should not mutate profile: %+v", profile)
	}

	fallbackInput := newBrowserStartInput(profile.ProfileId, nil, nil, false, false, false, "missing-proxy", "http://127.0.0.1:38080")
	effectiveProxy, bridgeRef, releaseBridge, err = app.resolveBrowserStartProxy(fallbackInput, profile)
	if err != nil {
		t.Fatalf("fallback temporary proxy returned error: %v", err)
	}
	if effectiveProxy != "http://127.0.0.1:38080" {
		t.Fatalf("expected fallback temporary proxy config, got %q", effectiveProxy)
	}
	if bridgeRef.valid() || releaseBridge {
		t.Fatalf("fallback HTTP proxy should not acquire bridge: ref=%+v release=%v", bridgeRef, releaseBridge)
	}
	if profile.ProxyId != "stored-proxy" || profile.ProxyConfig != "http://127.0.0.1:18080" {
		t.Fatalf("fallback temporary proxy should not mutate profile: %+v", profile)
	}
}

func TestAppendLaunchTargetsUsesConfiguredDefaultStartURLs(t *testing.T) {
	t.Parallel()

	got := appendLaunchTargets([]string{"--disable-sync"}, nil, []string{"https://one.example/", "https://two.example/"}, false, false)
	want := []string{"--disable-sync", "https://one.example/", "https://two.example/"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("appendLaunchTargets mismatch: got=%v want=%v", got, want)
	}
}

func TestAppendLaunchTargetsUsesBlankPageWhenSessionRestoreDisabled(t *testing.T) {
	t.Parallel()

	got := appendLaunchTargets([]string{"--disable-sync"}, nil, []string{}, false, false)
	want := []string{"--disable-sync", "about:blank"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("appendLaunchTargets should fall back to about:blank: got=%v want=%v", got, want)
	}
}

func TestAppendLaunchTargetsPreservesSessionRestoreWhenEnabled(t *testing.T) {
	t.Parallel()

	got := appendLaunchTargets([]string{"--disable-sync"}, nil, []string{}, false, true)
	want := []string{"--disable-sync"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("appendLaunchTargets should preserve session restore behavior: got=%v want=%v", got, want)
	}
}

func TestBuildBrowserLaunchArgsUsesNoProxyServerForDirectProxy(t *testing.T) {
	t.Parallel()

	profile := &BrowserProfile{
		ProfileId: "profile-direct",
	}

	got := buildBrowserLaunchArgs(
		profile,
		`D:\profiles\direct`,
		9222,
		"direct://",
		nil,
		nil,
		nil,
		[]string{"about:blank"},
	)

	hasNoProxyServer := false
	for _, arg := range got {
		if arg == "--no-proxy-server" {
			hasNoProxyServer = true
		}
		if arg == "--proxy-server=direct://" {
			t.Fatalf("expected direct proxy launch args to avoid --proxy-server=direct://, got=%v", got)
		}
	}
	if !hasNoProxyServer {
		t.Fatalf("expected direct proxy to use --no-proxy-server, got=%v", got)
	}
}

func TestBuildBrowserLaunchArgsLoadsEnabledExtensions(t *testing.T) {
	t.Parallel()

	profile := &BrowserProfile{ProfileId: "profile-extension"}
	got := buildBrowserLaunchArgs(
		profile,
		`D:\profiles\extensions`,
		9333,
		"",
		[]string{`D:\extensions\a`, `D:\extensions\b`},
		nil,
		nil,
		[]string{"about:blank"},
	)

	wantLoad := `--load-extension=D:\extensions\a,D:\extensions\b`
	wantExcept := `--disable-extensions-except=D:\extensions\a,D:\extensions\b`
	if !containsString(got, wantLoad) || !containsString(got, wantExcept) {
		t.Fatalf("expected extension launch args, got=%v", got)
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
