package proxy

import "testing"

func TestClashVlessTransportKeepsTLSFingerprintAndHeaders(t *testing.T) {
	src := `
name: vless-ws
type: vless
server: edge.example.com
port: 443
uuid: 00000000-0000-0000-0000-000000000001
tls: true
sni: sni.example.com
skip-cert-verify: true
client-fingerprint: chrome
alpn:
  - h2
network: ws
ws-opts:
  path: /ray
  headers:
    Host: cdn.example.com
    User-Agent: ant
`

	_, outbound, err := ParseProxyNode(src)
	if err != nil {
		t.Fatalf("ParseProxyNode returned error: %v", err)
	}
	stream := outbound["streamSettings"].(map[string]interface{})
	tlsSettings := stream["tlsSettings"].(map[string]interface{})
	if tlsSettings["fingerprint"] != "chrome" {
		t.Fatalf("fingerprint = %v, want chrome", tlsSettings["fingerprint"])
	}
	alpn := tlsSettings["alpn"].([]string)
	if len(alpn) != 1 || alpn[0] != "h2" {
		t.Fatalf("alpn = %#v, want [h2]", alpn)
	}
	ws := stream["wsSettings"].(map[string]interface{})
	headers := ws["headers"].(map[string]interface{})
	if headers["Host"] != "cdn.example.com" || headers["User-Agent"] != "ant" {
		t.Fatalf("headers = %#v", headers)
	}
}

func TestClashVmessGRPCFallbackServiceName(t *testing.T) {
	src := `
name: vmess-grpc
type: vmess
server: edge.example.com
port: 443
uuid: 00000000-0000-0000-0000-000000000002
cipher: auto
tls: true
servername: sni.example.com
network: grpc
serviceName: svc
`

	_, outbound, err := ParseProxyNode(src)
	if err != nil {
		t.Fatalf("ParseProxyNode returned error: %v", err)
	}
	stream := outbound["streamSettings"].(map[string]interface{})
	grpc := stream["grpcSettings"].(map[string]interface{})
	if grpc["serviceName"] != "svc" {
		t.Fatalf("grpc serviceName = %v, want svc", grpc["serviceName"])
	}
}

func TestSingBoxHysteria2ClashKeepsTLSFingerprintAndCongestion(t *testing.T) {
	src := `
type: hysteria2
server: hy.example.com
port: 443
password: test-password
sni: sni.example.com
alpn:
  - h3
client-fingerprint: chrome
congestion-control: bbr
`

	out, err := BuildSingBoxOutbound(src)
	if err != nil {
		t.Fatalf("BuildSingBoxOutbound returned error: %v", err)
	}
	if out["congestion_control"] != "bbr" {
		t.Fatalf("congestion_control = %v, want bbr", out["congestion_control"])
	}
	tls := out["tls"].(map[string]interface{})
	alpn := tls["alpn"].([]string)
	if len(alpn) != 1 || alpn[0] != "h3" {
		t.Fatalf("tls.alpn = %#v, want [h3]", alpn)
	}
	utls := tls["utls"].(map[string]interface{})
	if utls["fingerprint"] != "chrome" {
		t.Fatalf("utls fingerprint = %v, want chrome", utls["fingerprint"])
	}
}

func TestSingBoxTUICClashKeepsServernameFingerprintAndCongestion(t *testing.T) {
	src := `
type: tuic
server: tuic.example.com
port: 443
uuid: 00000000-0000-0000-0000-000000000003
password: test-password
servername: sni.example.com
client-fingerprint: chrome
congestion-controller: cubic
`

	out, err := BuildSingBoxOutbound(src)
	if err != nil {
		t.Fatalf("BuildSingBoxOutbound returned error: %v", err)
	}
	if out["congestion_control"] != "cubic" {
		t.Fatalf("congestion_control = %v, want cubic", out["congestion_control"])
	}
	tls := out["tls"].(map[string]interface{})
	if tls["server_name"] != "sni.example.com" {
		t.Fatalf("tls.server_name = %v, want sni.example.com", tls["server_name"])
	}
	utls := tls["utls"].(map[string]interface{})
	if utls["fingerprint"] != "chrome" {
		t.Fatalf("utls fingerprint = %v, want chrome", utls["fingerprint"])
	}
}
