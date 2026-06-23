package proxy

import (
	"ant-chrome/backend/internal/config"
	"encoding/json"
	"os"
	"testing"
)

func TestDefaultSingBoxDNSConfigUsesPublicIPv4Servers(t *testing.T) {
	dns := defaultSingBoxDNSConfig()
	if dns["final"] != "public-dns" {
		t.Fatalf("dns.final = %v, want public-dns", dns["final"])
	}
	if dns["strategy"] != "ipv4_only" {
		t.Fatalf("dns.strategy = %v, want ipv4_only", dns["strategy"])
	}
	servers, ok := dns["servers"].([]interface{})
	if !ok || len(servers) < 2 {
		t.Fatalf("dns.servers = %#v, want at least two servers", dns["servers"])
	}
	first, ok := servers[0].(map[string]interface{})
	if !ok {
		t.Fatalf("first dns server is %T, want map", servers[0])
	}
	if first["type"] != "udp" || first["server"] != "223.5.5.5" {
		t.Fatalf("first dns server = %#v", first)
	}
}

func TestDefaultXrayDNSConfigUsesPublicServers(t *testing.T) {
	dns := defaultXrayDNSConfig()
	servers, ok := dns["servers"].([]interface{})
	if !ok || len(servers) != 2 {
		t.Fatalf("dns.servers = %#v, want two servers", dns["servers"])
	}
	if servers[0] != "223.5.5.5" || servers[1] != "119.29.29.29" {
		t.Fatalf("dns.servers = %#v", servers)
	}
}

func TestSingBoxRouteUsesDefaultDomainResolver(t *testing.T) {
	appConfig := config.DefaultConfig()
	appConfig.Browser.UserDataRoot = t.TempDir()
	m := &SingBoxManager{Config: appConfig, AppRoot: t.TempDir()}
	cfgPath, err := m.buildConfig("dns-route-test", map[string]interface{}{"type": "direct", "tag": "proxy-out"}, 12345)
	if err != nil {
		t.Fatalf("buildConfig returned error: %v", err)
	}
	var generatedConfig map[string]interface{}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config failed: %v", err)
	}
	if err := json.Unmarshal(data, &generatedConfig); err != nil {
		t.Fatalf("decode config failed: %v", err)
	}
	route, ok := generatedConfig["route"].(map[string]interface{})
	if !ok {
		t.Fatalf("route is %T, want map", generatedConfig["route"])
	}
	if route["default_domain_resolver"] != "public-dns" {
		t.Fatalf("default_domain_resolver = %v", route["default_domain_resolver"])
	}
}
