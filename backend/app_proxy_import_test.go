package backend

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testClashSubscriptionYAML = `
proxies:
  - name: test-node
    type: http
    server: example.com
    port: 8080
`

func TestBrowserProxyFetchClashByURLFallbackAfterHTTPStatus(t *testing.T) {
	var seenUserAgents []string
	var seenAccept string
	var seenCacheControl string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenUserAgents = append(seenUserAgents, r.Header.Get("User-Agent"))
		if len(seenUserAgents) == 1 {
			seenAccept = r.Header.Get("Accept")
			seenCacheControl = r.Header.Get("Cache-Control")
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		fmt.Fprint(w, testClashSubscriptionYAML)
	}))
	defer server.Close()

	result, err := (&App{}).BrowserProxyFetchClashByURL(server.URL + "/sub?token=test-token")
	if err != nil {
		t.Fatalf("BrowserProxyFetchClashByURL returned error: %v", err)
	}
	if got := result["proxyCount"]; got != 1 {
		t.Fatalf("proxyCount = %v, want 1", got)
	}
	if len(seenUserAgents) != 2 {
		t.Fatalf("request count = %d, want 2", len(seenUserAgents))
	}
	if seenUserAgents[0] != clashSubscriptionUserAgents[0] {
		t.Fatalf("first User-Agent = %q, want %q", seenUserAgents[0], clashSubscriptionUserAgents[0])
	}
	if seenUserAgents[1] != clashSubscriptionUserAgents[1] {
		t.Fatalf("second User-Agent = %q, want %q", seenUserAgents[1], clashSubscriptionUserAgents[1])
	}
	if seenAccept != "application/yaml,text/yaml,text/plain,*/*" {
		t.Fatalf("Accept = %q", seenAccept)
	}
	if seenCacheControl != "no-cache" {
		t.Fatalf("Cache-Control = %q", seenCacheControl)
	}
}

func TestBrowserProxyFetchClashByURLFallbackAfterHTMLContent(t *testing.T) {
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, "<html><body>client not supported</body></html>")
			return
		}
		fmt.Fprint(w, testClashSubscriptionYAML)
	}))
	defer server.Close()

	result, err := (&App{}).BrowserProxyFetchClashByURL(server.URL)
	if err != nil {
		t.Fatalf("BrowserProxyFetchClashByURL returned error: %v", err)
	}
	if got := result["proxyCount"]; got != 1 {
		t.Fatalf("proxyCount = %v, want 1", got)
	}
	if requestCount != 2 {
		t.Fatalf("request count = %d, want 2", requestCount)
	}
}

func TestBrowserProxyFetchClashByURLAllFallbackErrorsHideURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	rawURL := server.URL + "/sub/path?token=secret-token"
	_, err := (&App{}).BrowserProxyFetchClashByURL(rawURL)
	if err == nil {
		t.Fatal("BrowserProxyFetchClashByURL returned nil error, want failure")
	}
	errText := err.Error()
	for _, forbidden := range []string{rawURL, "secret-token", "token=", "/sub/path"} {
		if strings.Contains(errText, forbidden) {
			t.Fatalf("error %q leaked %q", errText, forbidden)
		}
	}
}

func TestNormalizeClashSubscriptionContentSupportsBase64URIList(t *testing.T) {
	raw := "anytls://secret@example.com:443?sni=sni.example.com&insecure=1#AnyTLS%20Node\n" +
		"trojan://pass@trojan.example.com:8443?sni=trojan-sni.example.com#Trojan%20Node"
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))
	content, payload, err := normalizeClashSubscriptionContent([]byte(encoded))
	if err != nil {
		t.Fatalf("normalizeClashSubscriptionContent returned error: %v", err)
	}
	if count := clashProxyCount(payload); count != 2 {
		t.Fatalf("proxy count = %d, want 2", count)
	}
	if !strings.Contains(content, "type: anytls") {
		t.Fatalf("content does not contain anytls node: %s", content)
	}
	if !strings.Contains(content, "type: trojan") {
		t.Fatalf("content does not contain trojan node: %s", content)
	}
}
