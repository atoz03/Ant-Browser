package backend

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestFingerprintLocaleOverrideUsesICULocale(t *testing.T) {
	got := fingerprintLocaleOverride([]string{
		"--fingerprint-platform=macos",
		"--lang=en-US",
		"--accept-lang=zh-CN,zh",
	})
	if got != "en_US" {
		t.Fatalf("fingerprintLocaleOverride() = %q, want en_US", got)
	}
}

func TestLocaleOverrideTargetType(t *testing.T) {
	for _, targetType := range []string{"page", "iframe", "worker", "shared_worker", "service_worker"} {
		if !localeOverrideTargetType(targetType) {
			t.Fatalf("localeOverrideTargetType(%q) = false", targetType)
		}
	}
	if localeOverrideTargetType("browser") {
		t.Fatal("browser target should not receive Emulation.setLocaleOverride")
	}
}

func TestMaintainLocaleOverrideConnectionAppliesLocaleBeforeResume(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	messages := make(chan []map[string]any, 1)
	ready := make(chan struct{})
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/json/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"webSocketDebuggerUrl":%q}`, "ws"+strings.TrimPrefix(server.URL, "http")+"/devtools/browser/test")
	})
	mux.HandleFunc("/devtools/browser/test", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		captured := make([]map[string]any, 0, 4)
		var message map[string]any
		if conn.ReadJSON(&message) != nil {
			return
		}
		captured = append(captured, message)
		_ = conn.WriteJSON(map[string]any{"id": captured[0]["id"], "result": map[string]any{}})
		_ = conn.WriteJSON(map[string]any{
			"method": "Target.attachedToTarget",
			"params": map[string]any{
				"sessionId":  "page-session",
				"targetInfo": map[string]any{"type": "page"},
			},
		})
		for len(captured) < 3 {
			message = nil
			if conn.ReadJSON(&message) != nil {
				return
			}
			captured = append(captured, message)
		}
		_ = conn.WriteJSON(map[string]any{"id": captured[2]["id"], "result": map[string]any{}})
		message = nil
		if conn.ReadJSON(&message) != nil {
			return
		}
		captured = append(captured, message)
		messages <- captured
	})

	_, portText, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- maintainLocaleOverrideConnection(ctx, port, "en_US", func() { close(ready) })
	}()

	select {
	case captured := <-messages:
		if method(captured[0]) != "Target.setAutoAttach" {
			t.Fatalf("first method = %q", method(captured[0]))
		}
		if method(captured[1]) != "Target.setAutoAttach" {
			t.Fatalf("second method = %q", method(captured[1]))
		}
		if method(captured[2]) != "Emulation.setLocaleOverride" || localeParam(captured[2]) != "en_US" {
			t.Fatalf("locale message = %#v", captured[2])
		}
		if method(captured[3]) != "Runtime.runIfWaitingForDebugger" {
			t.Fatalf("fourth method = %q", method(captured[3]))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for locale override messages")
	}
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("locale override did not report that auto attach is ready")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("locale override connection did not stop after cancellation")
	}
}

func method(message map[string]any) string {
	value, _ := message["method"].(string)
	return value
}

func localeParam(message map[string]any) string {
	params, _ := message["params"].(map[string]any)
	value, _ := params["locale"].(string)
	return value
}
