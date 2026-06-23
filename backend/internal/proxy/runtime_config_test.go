package proxy

import (
	"encoding/json"
	"os"
	"testing"

	"ant-chrome/backend/internal/config"
)

func TestXrayRuntimeConfigUsesWarningLogLevel(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Browser.UserDataRoot = t.TempDir()
	manager := &XrayManager{Config: cfg, AppRoot: t.TempDir()}

	cfgPath, err := manager.buildRuntimeConfigWithRoute(
		"log-level-test",
		[]interface{}{map[string]interface{}{"protocol": "freedom", "tag": "proxy-out"}},
		[]interface{}{},
		19092,
		"",
	)
	if err != nil {
		t.Fatalf("buildRuntimeConfigWithRoute returned error: %v", err)
	}

	runtimeConfig := readRuntimeConfigMap(t, cfgPath)
	logConfig := runtimeConfig["log"].(map[string]interface{})
	if got := logConfig["loglevel"]; got != "warning" {
		t.Fatalf("xray loglevel = %v, want warning", got)
	}
}

func TestXrayRuntimeConfigEnablesBrowserSniffing(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Browser.UserDataRoot = t.TempDir()
	manager := &XrayManager{Config: cfg, AppRoot: t.TempDir()}

	cfgPath, err := manager.buildRuntimeConfigWithRoute(
		"sniffing-test",
		[]interface{}{map[string]interface{}{"protocol": "freedom", "tag": "proxy-out"}},
		[]interface{}{},
		19094,
		"",
	)
	if err != nil {
		t.Fatalf("buildRuntimeConfigWithRoute returned error: %v", err)
	}

	runtimeConfig := readRuntimeConfigMap(t, cfgPath)
	inbounds := runtimeConfig["inbounds"].([]interface{})
	inbound := inbounds[0].(map[string]interface{})
	sniffing := inbound["sniffing"].(map[string]interface{})
	if sniffing["enabled"] != true {
		t.Fatalf("sniffing.enabled = %v, want true", sniffing["enabled"])
	}
	destOverride := sniffing["destOverride"].([]interface{})
	if len(destOverride) != 3 || destOverride[0] != "http" || destOverride[1] != "tls" || destOverride[2] != "quic" {
		t.Fatalf("sniffing.destOverride = %#v, want [http tls quic]", destOverride)
	}
}

func TestXrayRuntimeConfigRemovesDeprecatedAllowInsecure(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Browser.UserDataRoot = t.TempDir()
	manager := &XrayManager{Config: cfg, AppRoot: t.TempDir()}

	cfgPath, err := manager.buildRuntimeConfigWithRoute(
		"deprecated-field-test",
		[]interface{}{
			map[string]interface{}{
				"protocol": "trojan",
				"tag":      "proxy-out",
				"streamSettings": map[string]interface{}{
					"security": "tls",
					"tlsSettings": map[string]interface{}{
						"serverName":    "example.com",
						"allowInsecure": true,
					},
				},
			},
		},
		[]interface{}{},
		19095,
		"",
	)
	if err != nil {
		t.Fatalf("buildRuntimeConfigWithRoute returned error: %v", err)
	}

	runtimeConfig := readRuntimeConfigMap(t, cfgPath)
	outbounds := runtimeConfig["outbounds"].([]interface{})
	outbound := outbounds[0].(map[string]interface{})
	stream := outbound["streamSettings"].(map[string]interface{})
	tlsSettings := stream["tlsSettings"].(map[string]interface{})
	if _, ok := tlsSettings["allowInsecure"]; ok {
		t.Fatalf("runtime config must not include deprecated allowInsecure: %#v", tlsSettings)
	}
}

func TestXrayRuntimeConfigKeepsTrojanServersArray(t *testing.T) {
	node := "trojan://password@example.com:443?peer=sni.example.com&sni=sni.example.com&type=tcp"
	_, outbound, err := ParseProxyNode(node)
	if err != nil {
		t.Fatalf("ParseProxyNode returned error: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Browser.UserDataRoot = t.TempDir()
	manager := &XrayManager{Config: cfg, AppRoot: t.TempDir()}
	cfgPath, err := manager.buildRuntimeConfigWithRoute(
		"trojan-shape-test",
		[]interface{}{outbound},
		[]interface{}{},
		19096,
		"",
	)
	if err != nil {
		t.Fatalf("buildRuntimeConfigWithRoute returned error: %v", err)
	}

	runtimeConfig := readRuntimeConfigMap(t, cfgPath)
	outbounds := runtimeConfig["outbounds"].([]interface{})
	outboundConfig := outbounds[0].(map[string]interface{})
	settings := outboundConfig["settings"].(map[string]interface{})
	servers, ok := settings["servers"].([]interface{})
	if !ok || len(servers) != 1 {
		t.Fatalf("trojan settings.servers invalid: %#v", settings["servers"])
	}
	server := servers[0].(map[string]interface{})
	if server["address"] != "example.com" || server["port"] != float64(443) || server["password"] != "password" {
		t.Fatalf("trojan server invalid: %#v", server)
	}
	if _, ok := settings["address"]; ok {
		t.Fatalf("legacy flat trojan settings should not be present: %#v", settings)
	}
}

func TestSummarizeXrayErrorReturnsShortConfigReason(t *testing.T) {
	raw := `Xray 26.6.20 (Xray, Penetrates Everything.)
Failed to start: main: failed to load config files: [xray-config.json] > infra/conf: failed to build outbound config with tag proxy-out > infra/conf: Failed to build stream settings for outbound detour. > infra/conf: Failed to build TLS config. > The feature "allowInsecure" has been removed and migrated to "certificate". Please update your config(s) according to release note and documentation before removal.`

	got := summarizeXrayError(raw)
	want := "字段 allowInsecure 已被当前 Xray 移除"
	if got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
}

func TestSingBoxRuntimeConfigUsesWarnLogLevel(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Browser.UserDataRoot = t.TempDir()
	manager := &SingBoxManager{Config: cfg, AppRoot: t.TempDir()}

	cfgPath, err := manager.buildConfig("log-level-test", map[string]interface{}{"type": "direct", "tag": "proxy-out"}, 19093)
	if err != nil {
		t.Fatalf("buildConfig returned error: %v", err)
	}

	runtimeConfig := readRuntimeConfigMap(t, cfgPath)
	logConfig := runtimeConfig["log"].(map[string]interface{})
	if got := logConfig["level"]; got != "warn" {
		t.Fatalf("sing-box log level = %v, want warn", got)
	}
}

func readRuntimeConfigMap(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read runtime config failed: %v", err)
	}
	var runtimeConfig map[string]interface{}
	if err := json.Unmarshal(data, &runtimeConfig); err != nil {
		t.Fatalf("unmarshal runtime config failed: %v", err)
	}
	return runtimeConfig
}
