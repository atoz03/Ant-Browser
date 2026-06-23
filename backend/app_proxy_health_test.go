package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/config"
	"bufio"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBuildProxyIPHealthResultPreservesErrorSourceMetadata(t *testing.T) {
	result := buildProxyIPHealthResult("proxy-1", map[string]interface{}{
		"_source":    "trace",
		"_targetUrl": "https://example.invalid/trace",
		"_parser":    "cloudflare_trace",
	}, errors.New("request failed"))

	if result.Source != "trace" {
		t.Fatalf("source = %q, want trace", result.Source)
	}
	if result.Error != "request failed" {
		t.Fatalf("error = %q, want request failed", result.Error)
	}
	rawError, _ := result.RawData["error"].(string)
	if rawError != "request failed" {
		t.Fatalf("raw error = %q, want request failed", rawError)
	}
	if got, _ := result.RawData["_targetUrl"].(string); got != "https://example.invalid/trace" {
		t.Fatalf("target url = %q, want trace url", got)
	}
}

func TestProxySpeedWithConnectorHonorsXrayConnector(t *testing.T) {
	var requests atomic.Int32
	proxyURL, closeProxy := startBackendDelayedHTTPProxy(t, 10*time.Millisecond, &requests)
	t.Cleanup(closeProxy)

	cfg := config.DefaultConfig()
	app := NewApp(t.TempDir())
	app.config = cfg
	app.browserMgr = browser.NewManager(cfg, t.TempDir())

	result := app.testProxySpeedWithConnector(
		"proxy-1",
		[]BrowserProxy{{ProxyId: "proxy-1", ProxyConfig: proxyURL}},
		config.BrowserConnectorXray,
	)
	if !result.Ok {
		t.Fatalf("testProxySpeedWithConnector failed: %+v", result)
	}
	if result.Engine != "native" {
		t.Fatalf("engine = %q, want native", result.Engine)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want unified-delay HTTP request pair", requests.Load())
	}
}

func TestProxySpeedBatchConcurrencyDefaultsAreConservative(t *testing.T) {
	if defaultProxySpeedConcurrency != 5 {
		t.Fatalf("defaultProxySpeedConcurrency = %d, want 5", defaultProxySpeedConcurrency)
	}
	if maxProxySpeedConcurrency != 10 {
		t.Fatalf("maxProxySpeedConcurrency = %d, want 10", maxProxySpeedConcurrency)
	}
}

func TestProxySpeedWithXrayUsesSingBoxProtocolPath(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()
	app := NewApp(t.TempDir())
	app.config = cfg
	app.browserMgr = browser.NewManager(cfg, t.TempDir())

	result := app.testProxySpeedWithConnector(
		"hy2-proxy",
		[]BrowserProxy{{ProxyId: "hy2-proxy", ProxyConfig: "hysteria2://pass@example.com:443?sni=example.com"}},
		config.BrowserConnectorXray,
	)
	if result.Ok {
		t.Fatalf("hysteria2 speed test should fail without sing-box manager: %+v", result)
	}
	if result.Engine != "sing-box" {
		t.Fatalf("engine = %q, want sing-box", result.Engine)
	}
	if !strings.Contains(result.Error, "sing-box 管理器未初始化") {
		t.Fatalf("error = %q, want sing-box manager guidance", result.Error)
	}
}

func startBackendDelayedHTTPProxy(t *testing.T, delay time.Duration, requests *atomic.Int32) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleBackendDelayedHTTPProxyConn(conn, delay, requests)
		}
	}()
	return "http://" + listener.Addr().String(), func() {
		_ = listener.Close()
		<-done
	}
}

func handleBackendDelayedHTTPProxyConn(conn net.Conn, delay time.Duration, requests *atomic.Int32) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return
	}
	for {
		header, err := reader.ReadString('\n')
		if err != nil || strings.TrimSpace(header) == "" {
			break
		}
	}
	if strings.HasPrefix(line, "HEAD ") {
		requests.Add(1)
		time.Sleep(delay)
		_, _ = fmt.Fprint(conn, "HTTP/1.1 204 No Content\r\nContent-Length: 0\r\n\r\n")
	}
}
