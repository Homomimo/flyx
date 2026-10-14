package kernel

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite golden files")

const (
	testUUID       = "0b6e4c38-8f0e-4a3e-9d2d-7a1c2b3c4d5e"
	testRealityKey = "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA" // bytes 1..32, raw-url
)

func TestNormalizeFillsCredentials(t *testing.T) {
	in := Inbound{ID: 1, Protocol: "VLESS", Port: 443, Settings: Settings{Flow: FlowVision}, Security: Security{Type: SecurityReality}}
	if err := Normalize(TypeXray, &in); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if in.Protocol != ProtocolVLESS || in.Transport.Type != TransportTCP || in.Listen != "::" {
		t.Fatalf("defaults not applied: %+v", in)
	}
	if !uuidPattern.MatchString(in.Settings.UUID) {
		t.Fatalf("uuid not generated: %q", in.Settings.UUID)
	}
	if in.Settings.Flow != FlowVision {
		t.Fatalf("vision flow dropped on tcp+reality")
	}
	sec := in.Security
	if sec.SNI != DefaultRealitySNI || sec.RealityDest != DefaultRealitySNI+":443" {
		t.Fatalf("reality defaults: %+v", sec)
	}
	pub, err := RealityPublicKey(sec.RealityPrivateKey)
	if err != nil || pub != sec.RealityPublicKey {
		t.Fatalf("public key mismatch: %v", err)
	}
	if len(sec.RealityShortIDs) != 1 || len(sec.RealityShortIDs[0]) != 16 {
		t.Fatalf("short id: %v", sec.RealityShortIDs)
	}
}

func TestNormalizeRejectsUnsupportedCombos(t *testing.T) {
	cases := []struct {
		name string
		in   Inbound
	}{
		{"unknown protocol", Inbound{Protocol: "snell", Port: 1}},
		{"ws reality", Inbound{Protocol: ProtocolVLESS, Port: 1, Transport: Transport{Type: TransportWS}, Security: Security{Type: SecurityReality}}},
		{"trojan without tls", Inbound{Protocol: ProtocolTrojan, Port: 1, Security: Security{Type: SecurityNone}}},
		{"bad port", Inbound{Protocol: ProtocolVMess, Port: 70000}},
		{"tls without sni", Inbound{Protocol: ProtocolVMess, Port: 1, Security: Security{Type: SecurityTLS}}},
		{"bad ss2022 key", Inbound{Protocol: ProtocolShadowsocks, Port: 1, Settings: Settings{Method: DefaultSSMethod, Password: "short"}}},
		{"bad uuid", Inbound{Protocol: ProtocolVMess, Port: 1, Settings: Settings{UUID: "nope"}}},
		{"extra not object", Inbound{Protocol: ProtocolVMess, Port: 1, Extra: json.RawMessage(`[1]`)}},
		{"bad short id", Inbound{Protocol: ProtocolVLESS, Port: 1, Security: Security{Type: SecurityReality, RealityShortIDs: []string{"xyz"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.in
			if err := Normalize(TypeXray, &in); err == nil {
				t.Fatalf("expected error for %+v", tc.in)
			}
		})
	}
	in := Inbound{Protocol: ProtocolVLESS, Port: 1}
	if err := Normalize(TypeSnell, &in); err == nil {
		t.Fatal("unimplemented kernel must be rejected")
	}
}

func TestNormalizeDropsVisionWhereUnsupported(t *testing.T) {
	in := Inbound{Protocol: ProtocolVLESS, Port: 1, Transport: Transport{Type: TransportWS}, Security: Security{Type: SecurityNone}, Settings: Settings{Flow: FlowVision}}
	if err := Normalize(TypeXray, &in); err != nil {
		t.Fatal(err)
	}
	if in.Settings.Flow != "" {
		t.Fatalf("vision must be cleared on ws: %q", in.Settings.Flow)
	}
	if !strings.HasPrefix(in.Transport.Path, "/") {
		t.Fatalf("ws path default: %q", in.Transport.Path)
	}
}

func TestNormalizeSS2022Key(t *testing.T) {
	in := Inbound{Protocol: ProtocolShadowsocks, Port: 8388}
	if err := Normalize(TypeXray, &in); err != nil {
		t.Fatal(err)
	}
	key, err := base64.StdEncoding.DecodeString(in.Settings.Password)
	if err != nil || len(key) != 16 {
		t.Fatalf("ss2022 key: %q", in.Settings.Password)
	}
	if in.Network() != "tcp,udp" {
		t.Fatalf("ss network: %s", in.Network())
	}
}

func TestNormalizeSelfSignedCertKeptWhenSNIUnchanged(t *testing.T) {
	in := Inbound{Protocol: ProtocolTrojan, Port: 443, Security: Security{Type: SecurityTLS, SNI: "a.example.com"}}
	if err := Normalize(TypeXray, &in); err != nil {
		t.Fatal(err)
	}
	cert := in.Security.CertPEM
	if !certMatchesName(cert, "a.example.com") {
		t.Fatal("cert does not match sni")
	}
	if err := Normalize(TypeXray, &in); err != nil {
		t.Fatal(err)
	}
	if in.Security.CertPEM != cert {
		t.Fatal("cert regenerated although sni unchanged")
	}
	in.Security.SNI = "b.example.com"
	if err := Normalize(TypeXray, &in); err != nil {
		t.Fatal(err)
	}
	if in.Security.CertPEM == cert || !certMatchesName(in.Security.CertPEM, "b.example.com") {
		t.Fatal("cert must follow sni change")
	}
}

func TestRegenerateRotatesSecrets(t *testing.T) {
	in := Inbound{Protocol: ProtocolVLESS, Port: 1, Security: Security{Type: SecurityReality}}
	if err := Normalize(TypeXray, &in); err != nil {
		t.Fatal(err)
	}
	before := in
	if err := Regenerate(TypeXray, &in); err != nil {
		t.Fatal(err)
	}
	if in.Settings.UUID == before.Settings.UUID || in.Security.RealityPrivateKey == before.Security.RealityPrivateKey {
		t.Fatal("secrets not rotated")
	}
	if in.Security.SNI != before.Security.SNI {
		t.Fatal("shape must be kept")
	}
}

func goldenInbounds(t *testing.T) []Inbound {
	t.Helper()
	certPEM := "-----BEGIN CERTIFICATE-----\nMIIB\nAAAA\n-----END CERTIFICATE-----\n"
	keyPEM := "-----BEGIN EC PRIVATE KEY-----\nMHcC\n-----END EC PRIVATE KEY-----\n"
	list := []Inbound{
		{ID: 3, Protocol: ProtocolVMess, Port: 8443,
			Settings:  Settings{UUID: testUUID},
			Transport: Transport{Type: TransportWS, Path: "/ws", Host: "cdn.example.com"},
			Security:  Security{Type: SecurityTLS, SNI: "cdn.example.com", CertMode: "file", CertFile: "/etc/ssl/a.crt", KeyFile: "/etc/ssl/a.key"}},
		{ID: 1, Protocol: ProtocolVLESS, Port: 443,
			Settings: Settings{UUID: testUUID, Flow: FlowVision},
			Security: Security{Type: SecurityReality, SNI: "www.example.com", RealityPrivateKey: testRealityKey, RealityShortIDs: []string{"0123456789abcdef"}}},
		{ID: 2, Protocol: ProtocolShadowsocks, Port: 8388,
			Settings: Settings{Method: DefaultSSMethod, Password: "AAAAAAAAAAAAAAAAAAAAAA=="}},
		{ID: 4, Protocol: ProtocolTrojan, Port: 2053,
			Settings:  Settings{Password: "trojan-secret"},
			Transport: Transport{Type: TransportGRPC, ServiceName: "tun"},
			Security:  Security{Type: SecurityTLS, SNI: "t.example.com", ALPN: []string{"h2"}, CertPEM: certPEM, KeyPEM: keyPEM}},
		{ID: 5, Protocol: ProtocolVLESS, Port: 9443, Listen: "127.0.0.1",
			Settings:  Settings{UUID: testUUID},
			Transport: Transport{Type: TransportXHTTP, Path: "/x"},
			Security:  Security{Type: SecurityNone},
			Extra:     json.RawMessage(`{"sniffing":{"enabled":false},"streamSettings":{"sockopt":{"tcpFastOpen":true}}}`)},
	}
	for i := range list {
		if list[i].Security.Type == SecurityTLS && list[i].Security.CertPEM != "" {
			// Keep the fixed fake PEM: Normalize would replace a cert that
			// does not parse, so only validate the non-TLS parts here.
			saved := list[i].Security
			if err := Normalize(TypeXray, &list[i]); err != nil {
				t.Fatalf("normalize %d: %v", list[i].ID, err)
			}
			list[i].Security.CertPEM, list[i].Security.KeyPEM = saved.CertPEM, saved.KeyPEM
			continue
		}
		if err := Normalize(TypeXray, &list[i]); err != nil {
			t.Fatalf("normalize %d: %v", list[i].ID, err)
		}
	}
	return list
}

func TestRenderXrayGolden(t *testing.T) {
	out, err := Render(TypeXray, goldenInbounds(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 1 || out.Files[0].Name != "config.json" {
		t.Fatalf("files: %+v", out.Files)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(out.Files[0].Content), &parsed); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	assertGolden(t, "xray.golden.json", out.Files[0].Content)

	again, _ := Render(TypeXray, goldenInbounds(t))
	if again.Hash != out.Hash {
		t.Fatal("render must be deterministic")
	}
}

func TestRenderEmptyXray(t *testing.T) {
	out, err := Render(TypeXray, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Files[0].Content, `"inbounds": []`) {
		t.Fatalf("empty inbounds must render as []: %s", out.Files[0].Content)
	}
}

func TestResolveDownload(t *testing.T) {
	info, _ := Lookup(TypeXray)
	d, ok := ResolveDownload(info, "v25.1.1", "https://mirror.example/")
	if !ok {
		t.Fatal("xray must be downloadable")
	}
	if d["url"] != "https://mirror.example/https://github.com/XTLS/Xray-core/releases/download/v25.1.1/Xray-linux-{ARCH}.zip" {
		t.Fatalf("url: %v", d["url"])
	}
	latest, _ := ResolveDownload(info, "latest", "")
	if !strings.Contains(latest["url"].(string), "/releases/latest/download/") {
		t.Fatalf("latest url: %v", latest["url"])
	}
}

func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update): %v", err)
	}
	if string(want) != got {
		t.Fatalf("%s mismatch:\n--- got ---\n%s", name, got)
	}
}
