package browser

import (
	"encoding/binary"
	"net/url"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestBuildChromeExtensionDownloadURLUsesCurrentChromiumVersion(t *testing.T) {
	raw := BuildChromeExtensionDownloadURL("ddkjiahejlhfcafbddmgiahcphecmpfh")
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.Query().Get("prodversion"); got != extensionStoreProdVersion {
		t.Fatalf("prodversion = %q, want %q", got, extensionStoreProdVersion)
	}
	if parsed.Query().Get("prod") != "chromiumcrx" {
		t.Fatalf("prod = %q, want chromiumcrx", parsed.Query().Get("prod"))
	}
	if !strings.Contains(parsed.Query().Get("x"), "ddkjiahejlhfcafbddmgiahcphecmpfh") {
		t.Fatalf("x does not contain extension ID: %q", parsed.Query().Get("x"))
	}
}

func TestExtensionIDFromCRX3Package(t *testing.T) {
	crxID := []byte{0xdd, 0xa9, 0x80, 0x74, 0x9b, 0x75, 0x20, 0x51, 0x33, 0xc6, 0x80, 0x72, 0xf7, 0x42, 0xcf, 0x57}
	signedData := protowire.AppendTag(nil, 1, protowire.BytesType)
	signedData = protowire.AppendBytes(signedData, crxID)
	header := protowire.AppendTag(nil, 10000, protowire.BytesType)
	header = protowire.AppendBytes(header, signedData)
	data := append([]byte("Cr24"), make([]byte, 8)...)
	binary.LittleEndian.PutUint32(data[4:8], 3)
	binary.LittleEndian.PutUint32(data[8:12], uint32(len(header)))
	data = append(data, header...)

	if got, want := extensionIDFromPackage(data), "nnkjiahejlhfcafbddmgiahcphecmpfh"; got != want {
		t.Fatalf("extensionIDFromPackage() = %q, want %q", got, want)
	}
}
