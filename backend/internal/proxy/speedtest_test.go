package proxy

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ant-chrome/backend/internal/config"
)

func TestProxyConfigToMappingStandardProxy(t *testing.T) {
	t.Parallel()

	mapping, err := proxyConfigToMapping("http://user:pass@example.com:8080/path")
	if err != nil {
		t.Fatalf("proxyConfigToMapping returned error: %v", err)
	}

	if got := mapping["type"]; got != "http" {
		t.Fatalf("type = %v, want http", got)
	}
	if got := mapping["server"]; got != "example.com" {
		t.Fatalf("server = %v, want example.com", got)
	}
	if got := mapping["port"]; got != 8080 {
		t.Fatalf("port = %v, want 8080", got)
	}
	if got := mapping["username"]; got != "user" {
		t.Fatalf("username = %v, want user", got)
	}
	if got := mapping["password"]; got != "pass" {
		t.Fatalf("password = %v, want pass", got)
	}
}

func TestProxyConfigToMappingEscapedCredentials(t *testing.T) {
	t.Parallel()

	mapping, err := proxyConfigToMapping("http://user%40mail:p%40ss%3Aword@example.com:8080")
	if err != nil {
		t.Fatalf("proxyConfigToMapping returned error: %v", err)
	}
	if got := mapping["username"]; got != "user@mail" {
		t.Fatalf("username = %v, want user@mail", got)
	}
	if got := mapping["password"]; got != "p@ss:word" {
		t.Fatalf("password = %v, want p@ss:word", got)
	}
}

func TestProxyEndpointDropsCredentials(t *testing.T) {
	t.Parallel()

	endpoint, err := proxyEndpoint("http://user:pass@example.com:8080")
	if err != nil {
		t.Fatalf("proxyEndpoint returned error: %v", err)
	}
	if endpoint != "example.com:8080" {
		t.Fatalf("endpoint = %q, want example.com:8080", endpoint)
	}
}

func TestProxyConfigToMappingClashYAML(t *testing.T) {
	t.Parallel()

	src := "proxies:\n  - type: vmess\n    server: test.example.com\n    port: 443\n"
	mapping, err := proxyConfigToMapping(src)
	if err != nil {
		t.Fatalf("proxyConfigToMapping returned error: %v", err)
	}

	if got := mapping["type"]; got != "vmess" {
		t.Fatalf("type = %v, want vmess", got)
	}
	if got := mapping["server"]; got != "test.example.com" {
		t.Fatalf("server = %v, want test.example.com", got)
	}
	if got := mapping["port"]; got != 443 {
		t.Fatalf("port = %v, want 443", got)
	}
	if got := mapping["name"]; got != "speedtest-proxy" {
		t.Fatalf("name = %v, want speedtest-proxy", got)
	}
}

func TestProxyConfigToMappingSSURI(t *testing.T) {
	t.Parallel()

	userinfo := base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:secret"))
	mapping, err := proxyConfigToMapping("ss://" + userinfo + "@ptxlv6-1.hxx.top:43001#node")
	if err != nil {
		t.Fatalf("proxyConfigToMapping returned error: %v", err)
	}
	if got := mapping["type"]; got != "ss" {
		t.Fatalf("type = %v, want ss", got)
	}
	if got := mapping["server"]; got != "ptxlv6-1.hxx.top" {
		t.Fatalf("server = %v, want ptxlv6-1.hxx.top", got)
	}
	if got := mapping["port"]; got != 43001 {
		t.Fatalf("port = %v, want 43001", got)
	}
	if got := mapping["cipher"]; got != "aes-128-gcm" {
		t.Fatalf("cipher = %v, want aes-128-gcm", got)
	}
	if got := mapping["password"]; got != "secret" {
		t.Fatalf("password = %v, want secret", got)
	}
}

func TestProxyEndpointSSURIIPv6(t *testing.T) {
	t.Parallel()

	raw := base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:secret@[2001:db8::1]:43001"))
	endpoint, err := proxyEndpoint("ss://" + raw)
	if err != nil {
		t.Fatalf("proxyEndpoint returned error: %v", err)
	}
	if endpoint != "[2001:db8::1]:43001" {
		t.Fatalf("endpoint = %q, want [2001:db8::1]:43001", endpoint)
	}
}

func TestProxyConfigToMappingUnsupportedURI(t *testing.T) {
	t.Parallel()

	if _, err := proxyConfigToMapping("vmess://example"); err == nil {
		t.Fatal("expected unsupported URI error")
	}
}

func TestDefaultProxyCheckURLsAreConfigured(t *testing.T) {
	t.Parallel()

	if strings.TrimSpace(DefaultSpeedTestURL) == "" {
		t.Fatalf("DefaultSpeedTestURL must not be empty")
	}
	if strings.TrimSpace(DefaultIPHealthURL) == "" {
		t.Fatalf("DefaultIPHealthURL must not be empty")
	}
}

func TestDefaultSpeedTestTimeoutsAreShort(t *testing.T) {
	t.Parallel()

	if DefaultSpeedTestConfig.Timeout != 3*time.Second {
		t.Fatalf("speed timeout = %s, want 3s", DefaultSpeedTestConfig.Timeout)
	}
	if DefaultSpeedTestConfig.TCPTimeout != 3*time.Second {
		t.Fatalf("speed tcp timeout = %s, want 3s", DefaultSpeedTestConfig.TCPTimeout)
	}
}

func TestSpeedTestDefaultsToXrayLightHTTPDelay(t *testing.T) {
	var requests atomic.Int32
	const responseDelay = 120 * time.Millisecond
	proxyURL, closeProxy := startDelayedConnectProxy(t, responseDelay, &requests)
	t.Cleanup(closeProxy)

	proxyID := "delayed-http-proxy"
	result := SpeedTest(
		proxyID,
		[]config.BrowserProxy{{ProxyId: proxyID, ProxyConfig: proxyURL}},
		nil,
		nil,
		&SpeedTestConfig{Timeout: 2 * time.Second, URLs: []string{"http://latency.test/generate_204"}},
	)
	if !result.Ok {
		t.Fatalf("SpeedTest failed: %+v", result)
	}
	if requests.Load() == 0 {
		t.Fatal("test proxy did not receive any speed-test request")
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want unified delay to perform two HEAD requests", requests.Load())
	}
	if result.Engine != "native" {
		t.Fatalf("engine = %q, want native", result.Engine)
	}
	if result.LatencyMs <= 0 || result.LatencyMs >= int64(responseDelay/time.Millisecond) {
		t.Fatalf("latency = %dms, want second unified-delay probe below first-connection delay", result.LatencyMs)
	}
}

func TestSpeedTestFallsBackAcrossTargets(t *testing.T) {
	var requests atomic.Int32
	proxyURL, closeProxy := startDelayedConnectProxy(t, 10*time.Millisecond, &requests)
	t.Cleanup(closeProxy)

	proxyID := "fallback-http-proxy"
	result := SpeedTestWithConnector(
		proxyID,
		[]config.BrowserProxy{{ProxyId: proxyID, ProxyConfig: proxyURL}},
		nil,
		nil,
		nil,
		config.BrowserConnectorXray,
		&SpeedTestConfig{Timeout: 2 * time.Second, URLs: []string{"http://latency.test/fail", "http://latency.test/generate_204"}},
	)
	if !result.Ok {
		t.Fatalf("SpeedTestWithConnector should fallback to second target: %+v", result)
	}
	if requests.Load() != 4 {
		t.Fatalf("requests = %d, want fallback to perform unified-delay HEAD pair per target", requests.Load())
	}
}

func TestSpeedTestTargetsDoNotIncludeRealConnectivityFallbacks(t *testing.T) {
	t.Parallel()

	targets := speedTestTargetURLs(&SpeedTestConfig{})
	if len(targets) != 1 {
		t.Fatalf("targets = %#v, want only default speed test URL", targets)
	}
	if targets[0] != DefaultSpeedTestURL {
		t.Fatalf("target = %q, want %q", targets[0], DefaultSpeedTestURL)
	}
	for _, target := range targets {
		if strings.Contains(target, "cloudflare") || strings.Contains(target, "msftconnecttest") {
			t.Fatalf("speed test target unexpectedly includes real-connectivity URL: %#v", targets)
		}
	}
}

func TestSpeedTestUsesSingBoxProtocolWhenXrayConnectorSelected(t *testing.T) {
	proxyID := "hy2-proxy"
	result := SpeedTestWithConnector(
		proxyID,
		[]config.BrowserProxy{{ProxyId: proxyID, ProxyConfig: "hysteria2://pass@example.com:443?sni=example.com"}},
		nil,
		nil,
		nil,
		config.BrowserConnectorXray,
		&SpeedTestConfig{Timeout: 10 * time.Millisecond, URLs: []string{"http://latency.test/generate_204"}},
	)
	if result.Ok {
		t.Fatalf("speed test should fail without sing-box manager, got success: %+v", result)
	}
	if result.Engine != "sing-box" {
		t.Fatalf("engine = %q, want sing-box; result=%+v", result.Engine, result)
	}
	if !strings.Contains(result.Error, "sing-box 管理器未初始化") {
		t.Fatalf("error = %q, want sing-box manager guidance", result.Error)
	}
}

func startDelayedConnectProxy(t *testing.T, delay time.Duration, requests *atomic.Int32) (string, func()) {
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
			go handleDelayedConnectProxyConn(conn, delay, requests)
		}
	}()
	return "http://" + listener.Addr().String(), func() {
		_ = listener.Close()
		<-done
	}
}

func handleDelayedConnectProxyConn(conn net.Conn, delay time.Duration, requests *atomic.Int32) {
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
	if strings.HasPrefix(line, "CONNECT ") {
		_, _ = fmt.Fprint(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
		line, err = reader.ReadString('\n')
		if err != nil {
			return
		}
		for {
			header, err := reader.ReadString('\n')
			if err != nil || strings.TrimSpace(header) == "" {
				break
			}
		}
	}
	for strings.HasPrefix(line, "HEAD ") {
		requestCount := requests.Add(1)
		if requestCount == 1 {
			time.Sleep(delay)
		} else {
			time.Sleep(10 * time.Millisecond)
		}
		statusLine := "HTTP/1.1 204 No Content"
		if strings.Contains(line, "/fail") {
			statusLine = "HTTP/1.1 500 Internal Server Error"
		}
		_, _ = fmt.Fprintf(conn, "%s\r\nContent-Length: 0\r\nConnection: keep-alive\r\n\r\n", statusLine)
		line, err = reader.ReadString('\n')
		if err != nil {
			return
		}
		for {
			header, err := reader.ReadString('\n')
			if err != nil || strings.TrimSpace(header) == "" {
				break
			}
		}
	}
}
