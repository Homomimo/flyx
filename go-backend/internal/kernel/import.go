package kernel

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ImportedNode is a proxy node the panel does not deploy: it only records
// its parameters so relays and subscriptions can point at it.
type ImportedNode struct {
	Name    string
	Host    string
	Port    int
	Inbound Inbound
}

// ParseShareLink parses one share link. Only Shadowsocks (SIP002 and the
// legacy all-base64 form) is supported for now.
func ParseShareLink(raw string) (ImportedNode, error) {
	raw = strings.TrimSpace(raw)
	scheme, _, ok := strings.Cut(raw, "://")
	if !ok {
		return ImportedNode{}, errors.New("不是分享链接")
	}
	switch strings.ToLower(scheme) {
	case "ss":
		return parseShadowsocksLink(raw[len(scheme)+3:])
	default:
		return ImportedNode{}, fmt.Errorf("暂不支持 %s:// 链接，目前只支持 ss://", scheme)
	}
}

func parseShadowsocksLink(rest string) (ImportedNode, error) {
	var name string
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		frag, err := url.PathUnescape(rest[i+1:])
		if err != nil {
			frag = rest[i+1:]
		}
		name, rest = strings.TrimSpace(frag), rest[:i]
	}
	var query string
	if i := strings.IndexByte(rest, '?'); i >= 0 {
		rest, query = rest[:i], rest[i+1:]
	}
	rest = strings.TrimSuffix(rest, "/")
	if q, err := url.ParseQuery(query); err == nil && q.Get("plugin") != "" {
		return ImportedNode{}, errors.New("暂不支持带插件（obfs / v2ray-plugin / shadow-tls）的 ss 链接")
	}

	var userinfo, hostport string
	if at := strings.LastIndexByte(rest, '@'); at >= 0 {
		// SIP002: userinfo is base64url(method:password), or percent-encoded
		// plain text for 2022 ciphers.
		userinfo, hostport = rest[:at], rest[at+1:]
		if decoded, err := decodeBase64Loose(userinfo); err == nil && strings.Contains(string(decoded), ":") {
			userinfo = string(decoded)
		} else if plain, err := url.PathUnescape(userinfo); err == nil {
			userinfo = plain
		}
	} else {
		// Legacy: base64(method:password@host:port).
		decoded, err := decodeBase64Loose(rest)
		if err != nil {
			return ImportedNode{}, errors.New("ss 链接格式不正确")
		}
		at := strings.LastIndexByte(string(decoded), '@')
		if at < 0 {
			return ImportedNode{}, errors.New("ss 链接格式不正确")
		}
		userinfo, hostport = string(decoded[:at]), string(decoded[at+1:])
	}

	method, password, ok := strings.Cut(userinfo, ":")
	method = strings.ToLower(strings.TrimSpace(method))
	if !ok || password == "" {
		return ImportedNode{}, errors.New("ss 链接缺少加密方式或密码")
	}
	if !containsString(ssMethods, method) {
		return ImportedNode{}, fmt.Errorf("不支持的加密方式 %s", method)
	}
	host, port, err := ParseImportHostPort(hostport)
	if err != nil {
		return ImportedNode{}, err
	}
	if name == "" {
		name = host
	}
	return ImportedNode{
		Name: name,
		Host: host,
		Port: port,
		Inbound: Inbound{
			Protocol:  ProtocolShadowsocks,
			Listen:    host,
			Port:      port,
			Settings:  Settings{Method: method, Password: password},
			Transport: Transport{Type: TransportTCP},
			Security:  Security{Type: SecurityNone},
		},
	}, nil
}

// ParseImportHostPort validates the address of an imported node: a public
// IP or a hostname, plus a port.
func ParseImportHostPort(hostport string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(strings.TrimSpace(hostport))
	if err != nil {
		return "", 0, errors.New("地址格式应为 主机:端口")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return "", 0, errors.New("端口不正确")
	}
	if err := ValidateImportHost(host); err != nil {
		return "", 0, err
	}
	return host, port, nil
}

// ValidateImportHost accepts a routable IP or a hostname.
func ValidateImportHost(host string) error {
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsUnspecified() || ip.IsLoopback() {
			return errors.New("地址不能是本机或 0.0.0.0")
		}
		return nil
	}
	if host == "" || len(host) > 253 || !hostPattern.MatchString(host) || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return fmt.Errorf("地址 %q 不正确", host)
	}
	return nil
}

func decodeBase64Loose(s string) ([]byte, error) {
	s = strings.TrimRight(strings.TrimSpace(s), "=")
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawStdEncoding.DecodeString(s)
}
