package proxy

import (
	"ant-chrome/backend/internal/config"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildProxyDiagnosticAuthenticatedSocks5UsesXrayBridge(t *testing.T) {
	cfg := &config.Config{}
	cfg.Browser.UserDataRoot = t.TempDir()
	manager := NewXrayManager(cfg, "")
	t.Cleanup(manager.StopAll)

	proxies := []config.BrowserProxy{{
		ProxyId:     "p1",
		ProxyName:   "auth-socks",
		ProxyConfig: "socks5://user:secret@127.0.0.1:1080",
		DnsServers:  "1.1.1.1",
	}}

	diag := BuildProxyDiagnostic("", proxies, "p1", BuildDiagnosticOptions{XrayMgr: manager})
	if !diag.Ok {
		t.Fatalf("expected diagnostic ok, errors=%v", diag.Errors)
	}
	if diag.Engine != "xray" {
		t.Fatalf("engine = %s, want xray", diag.Engine)
	}
	if diag.NodeKey == "" {
		t.Fatal("expected node key")
	}
	if !strings.Contains(diag.RawConfigMasked, "***") || strings.Contains(diag.RawConfigMasked, "secret") {
		t.Fatalf("raw config was not masked: %s", diag.RawConfigMasked)
	}
	if len(diag.Outbounds) != 1 || len(diag.Routes) != 1 {
		t.Fatalf("unexpected bridge plan: outbounds=%d routes=%d", len(diag.Outbounds), len(diag.Routes))
	}
	if diag.Runtime.WorkDir == "" || diag.Runtime.ConfigPath != filepath.Join(diag.Runtime.WorkDir, "xray-config.json") {
		t.Fatalf("unexpected runtime paths: %+v", diag.Runtime)
	}
}

func TestBuildProxyDiagnosticSingBoxMasksSecrets(t *testing.T) {
	cfg := &config.Config{}
	cfg.Browser.UserDataRoot = t.TempDir()
	manager := NewSingBoxManager(cfg, "")
	src := "hysteria2://pass123@example.com:443?sni=example.com&obfs-password=obfs-secret&insecure=1"

	diag := BuildProxyDiagnostic(src, nil, "", BuildDiagnosticOptions{SingBoxMgr: manager})
	if !diag.Ok {
		t.Fatalf("expected diagnostic ok, errors=%v", diag.Errors)
	}
	if diag.Engine != "sing-box" {
		t.Fatalf("engine = %s, want sing-box", diag.Engine)
	}
	if strings.Contains(diag.RawConfigMasked, "obfs-secret") {
		t.Fatalf("query secret was not masked: %s", diag.RawConfigMasked)
	}
	obfs, _ := diag.Outbound["obfs"].(map[string]interface{})
	if diag.Outbound["password"] != "***" || obfs["password"] != "***" {
		t.Fatalf("outbound secrets were not masked: %+v", diag.Outbound)
	}
	if diag.Runtime.ConfigPath != filepath.Join(diag.Runtime.WorkDir, "singbox-config.json") {
		t.Fatalf("unexpected runtime paths: %+v", diag.Runtime)
	}
}

func TestBuildProxyDiagnosticStandardProxyDoesNotBridge(t *testing.T) {
	diag := BuildProxyDiagnostic("http://127.0.0.1:8080", nil, "", BuildDiagnosticOptions{})
	if !diag.Ok {
		t.Fatalf("expected diagnostic ok, errors=%v", diag.Errors)
	}
	if diag.Engine != "none" {
		t.Fatalf("engine = %s, want none", diag.Engine)
	}
	if diag.StandardProxy != "http://127.0.0.1:8080" {
		t.Fatalf("standardProxy = %s", diag.StandardProxy)
	}
	if len(diag.Outbounds) != 0 || diag.Runtime.WorkDir != "" {
		t.Fatalf("standard proxy should not have bridge plan: %+v", diag)
	}
}

func TestBuildProxyDiagnosticMissingProxy(t *testing.T) {
	diag := BuildProxyDiagnostic("", []config.BrowserProxy{{ProxyId: "p1", ProxyConfig: "direct://"}}, "missing", BuildDiagnosticOptions{})
	if diag.Ok {
		t.Fatal("expected missing proxy diagnostic to fail")
	}
	if diag.Found {
		t.Fatal("expected Found=false")
	}
	if len(diag.Errors) != 1 || !strings.Contains(diag.Errors[0], "不存在") {
		t.Fatalf("unexpected errors: %v", diag.Errors)
	}
}
