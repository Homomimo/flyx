package kernel

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestParseShadowsocksLinks(t *testing.T) {
	legacy := "ss://" + base64.StdEncoding.EncodeToString([]byte("aes-256-gcm:pa:ss@203.0.113.5:8388")) + "#%E9%A6%99%E6%B8%AF"
	sip002 := "ss://" + base64.RawURLEncoding.EncodeToString([]byte("chacha20-ietf-poly1305:secret")) + "@example.com:443/#HK%201"
	ss2022 := "ss://2022-blake3-aes-128-gcm:MTIzNDU2Nzg5MDEyMzQ1Ng%3D%3D@[2001:db8::1]:9000#v6"

	cases := []struct {
		link, name, host, method, password string
		port                               int
	}{
		{legacy, "香港", "203.0.113.5", "aes-256-gcm", "pa:ss", 8388},
		{sip002, "HK 1", "example.com", "chacha20-ietf-poly1305", "secret", 443},
		{ss2022, "v6", "2001:db8::1", "2022-blake3-aes-128-gcm", "MTIzNDU2Nzg5MDEyMzQ1Ng==", 9000},
	}
	for _, tc := range cases {
		n, err := ParseShareLink(tc.link)
		if err != nil {
			t.Fatalf("%s: %v", tc.link, err)
		}
		s := n.Inbound.Settings
		if n.Name != tc.name || n.Host != tc.host || n.Port != tc.port || s.Method != tc.method || s.Password != tc.password {
			t.Fatalf("%s parsed as %+v", tc.link, n)
		}
		// Re-exported link must parse back to the same parameters.
		uri, ok := ShareURI(Endpoint{Name: n.Name, Host: n.Host, Port: n.Port, Inbound: n.Inbound})
		if !ok {
			t.Fatalf("no share link for %+v", n)
		}
		back, err := ParseShareLink(uri)
		if err != nil || back.Inbound.Settings != s || back.Host != n.Host || back.Port != n.Port || back.Name != n.Name {
			t.Fatalf("round trip %s -> %s -> %+v (%v)", tc.link, uri, back, err)
		}
	}

	bad := map[string]string{
		"vmess://abc": "只支持 ss://",
		"ss://" + base64.RawURLEncoding.EncodeToString([]byte("rc4-md5:x")) + "@1.2.3.4:80":                                      "不支持的加密方式",
		"ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:x")) + "@127.0.0.1:80":                                "本机",
		"ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:x")) + "@1.2.3.4:70000":                               "端口",
		"ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:x")) + "@1.2.3.4:80/?plugin=obfs-local%3Bobfs%3Dhttp": "插件",
		"hello": "不是分享链接",
	}
	for link, want := range bad {
		if _, err := ParseShareLink(link); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: want error %q, got %v", link, want, err)
		}
	}
}
