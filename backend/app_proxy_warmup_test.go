package backend

import (
	"ant-chrome/backend/internal/config"
	"testing"
)

func TestWarmupProxyBridgeDirectProxy(t *testing.T) {
	t.Parallel()

	app := &App{}
	result := app.warmupProxyBridge("direct", "", []BrowserProxy{{ProxyId: "direct", ProxyConfig: "direct://"}})
	if !result.Ok {
		t.Fatalf("direct warmup failed: %s", result.Error)
	}
	if result.Engine != "direct" {
		t.Fatalf("engine = %q, want direct", result.Engine)
	}
	if result.SocksURL != "" {
		t.Fatalf("direct warmup socks url = %q, want empty", result.SocksURL)
	}
}

func TestWarmupProxyBridgeStandardProxyDoesNotRequireBridge(t *testing.T) {
	t.Parallel()

	app := &App{}
	result := app.warmupProxyBridge("http", "", []BrowserProxy{{ProxyId: "http", ProxyConfig: "http://127.0.0.1:8080"}})
	if !result.Ok {
		t.Fatalf("standard proxy warmup failed: %s", result.Error)
	}
	if result.Engine != "native" {
		t.Fatalf("engine = %q, want native", result.Engine)
	}
}

func TestWarmupProxyBridgeMissingProxyConfig(t *testing.T) {
	t.Parallel()

	app := &App{}
	result := app.warmupProxyBridge("missing", "", []config.BrowserProxy{{ProxyId: "other", ProxyConfig: "direct://"}})
	if result.Ok {
		t.Fatalf("missing proxy config unexpectedly succeeded")
	}
	if result.Error == "" {
		t.Fatalf("missing proxy config should return an error")
	}
}

func TestResolveProxyConfigForApp(t *testing.T) {
	t.Parallel()

	proxies := []BrowserProxy{{ProxyId: "p1", ProxyConfig: "direct://"}}
	if got := resolveProxyConfigForApp("", proxies, "p1"); got != "direct://" {
		t.Fatalf("resolveProxyConfigForApp() = %q", got)
	}
	if got := resolveProxyConfigForApp("http://127.0.0.1:8080", proxies, "missing"); got != "http://127.0.0.1:8080" {
		t.Fatalf("fallback config = %q", got)
	}
}
