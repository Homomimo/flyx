package kernel

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Endpoint is one address a client can reach an inbound at: the landing node
// itself, or the entry of a relay forward in front of it.
type Endpoint struct {
	Name    string
	Host    string
	Port    int
	Inbound Inbound
}

const (
	FormatBase64  = "base64"
	FormatClash   = "clash"
	FormatSingbox = "singbox"
	FormatSurge   = "surge"
)

func (e Endpoint) selfSigned() bool {
	return e.Inbound.Security.Type == SecurityTLS && e.Inbound.Security.CertMode != "file"
}

func hostPort(host string, port int) string {
	return net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(port))
}

// ShareURI renders the de-facto standard share link (v2rayN / Shadowrocket /
// NekoBox style) for an endpoint.
func ShareURI(e Endpoint) (string, bool) {
	in := e.Inbound
	switch in.Protocol {
	case ProtocolVLESS, ProtocolTrojan:
		q := url.Values{}
		q.Set("type", in.Transport.Type)
		q.Set("security", in.Security.Type)
		if in.Protocol == ProtocolVLESS {
			q.Set("encryption", "none")
			if in.Settings.Flow != "" {
				q.Set("flow", in.Settings.Flow)
			}
		}
		addTransportQuery(q, in.Transport)
		addSecurityQuery(q, e)
		user := in.Settings.UUID
		if in.Protocol == ProtocolTrojan {
			user = url.PathEscape(in.Settings.Password)
		}
		return fmt.Sprintf("%s://%s@%s?%s#%s", in.Protocol, user, hostPort(e.Host, e.Port),
			q.Encode(), url.PathEscape(e.Name)), true
	case ProtocolVMess:
		v := map[string]interface{}{
			"v": "2", "ps": e.Name, "add": strings.Trim(e.Host, "[]"), "port": strconv.Itoa(e.Port),
			"id": in.Settings.UUID, "aid": "0", "scy": "auto", "net": in.Transport.Type,
			"type": "none", "host": in.Transport.Host, "path": in.Transport.Path, "tls": "",
		}
		if in.Transport.Type == TransportGRPC {
			v["path"] = in.Transport.ServiceName
			v["type"] = "gun"
		}
		if in.Security.Type == SecurityTLS {
			v["tls"] = "tls"
			v["sni"] = in.Security.SNI
			v["fp"] = in.Security.Fingerprint
			if len(in.Security.ALPN) > 0 {
				v["alpn"] = strings.Join(in.Security.ALPN, ",")
			}
			if e.selfSigned() {
				v["allowInsecure"] = "1"
			}
		}
		b, _ := json.Marshal(v)
		return "vmess://" + base64.StdEncoding.EncodeToString(b), true
	case ProtocolShadowsocks:
		userinfo := base64.RawURLEncoding.EncodeToString([]byte(in.Settings.Method + ":" + in.Settings.Password))
		if SS2022KeyLen(in.Settings.Method) > 0 {
			// SIP022: 2022 ciphers use percent-encoded plain userinfo.
			userinfo = url.QueryEscape(in.Settings.Method) + ":" + url.QueryEscape(in.Settings.Password)
		}
		return fmt.Sprintf("ss://%s@%s#%s", userinfo, hostPort(e.Host, e.Port), url.PathEscape(e.Name)), true
	}
	return "", false
}

func addTransportQuery(q url.Values, t Transport) {
	switch t.Type {
	case TransportWS:
		q.Set("path", t.Path)
		if t.Host != "" {
			q.Set("host", t.Host)
		}
	case TransportXHTTP:
		q.Set("path", t.Path)
		q.Set("mode", t.Mode)
		if t.Host != "" {
			q.Set("host", t.Host)
		}
	case TransportGRPC:
		q.Set("serviceName", t.ServiceName)
		q.Set("mode", "gun")
	}
}

func addSecurityQuery(q url.Values, e Endpoint) {
	sec := e.Inbound.Security
	switch sec.Type {
	case SecurityReality:
		q.Set("sni", sec.SNI)
		q.Set("fp", sec.Fingerprint)
		q.Set("pbk", sec.RealityPublicKey)
		if len(sec.RealityShortIDs) > 0 {
			q.Set("sid", sec.RealityShortIDs[0])
		}
	case SecurityTLS:
		q.Set("sni", sec.SNI)
		q.Set("fp", sec.Fingerprint)
		if len(sec.ALPN) > 0 {
			q.Set("alpn", strings.Join(sec.ALPN, ","))
		}
		if e.selfSigned() {
			q.Set("allowInsecure", "1")
		}
	}
}

// ClashProxy renders a Mihomo (Clash Meta) proxy entry. XHTTP is skipped
// because Mihomo support for it is not stable across client versions.
func ClashProxy(e Endpoint) (yamlNode, bool) {
	in := e.Inbound
	if in.Transport.Type == TransportXHTTP {
		return nil, false
	}
	p := yamlNode{
		{"name", e.Name},
		{"type", map[string]string{ProtocolShadowsocks: "ss"}[in.Protocol]},
		{"server", strings.Trim(e.Host, "[]")},
		{"port", e.Port},
		{"udp", true},
	}
	if p[1].Value == "" {
		p[1].Value = in.Protocol
	}
	switch in.Protocol {
	case ProtocolVLESS:
		p = append(p, yamlField{"uuid", in.Settings.UUID})
		if in.Settings.Flow != "" {
			p = append(p, yamlField{"flow", in.Settings.Flow})
		}
	case ProtocolVMess:
		p = append(p, yamlField{"uuid", in.Settings.UUID}, yamlField{"alterId", 0}, yamlField{"cipher", "auto"})
	case ProtocolTrojan:
		p = append(p, yamlField{"password", in.Settings.Password})
	case ProtocolShadowsocks:
		return append(p, yamlField{"cipher", in.Settings.Method}, yamlField{"password", in.Settings.Password}), true
	default:
		return nil, false
	}

	p = append(p, yamlField{"network", in.Transport.Type})
	switch in.Transport.Type {
	case TransportWS:
		ws := yamlNode{{"path", in.Transport.Path}}
		if in.Transport.Host != "" {
			ws = append(ws, yamlField{"headers", yamlNode{{"Host", in.Transport.Host}}})
		}
		p = append(p, yamlField{"ws-opts", ws})
	case TransportGRPC:
		p = append(p, yamlField{"grpc-opts", yamlNode{{"grpc-service-name", in.Transport.ServiceName}}})
	}

	sec := in.Security
	if sec.Type != SecurityNone {
		if in.Protocol != ProtocolTrojan {
			p = append(p, yamlField{"tls", true})
		}
		sniKey := "servername"
		if in.Protocol == ProtocolTrojan {
			sniKey = "sni"
		}
		p = append(p, yamlField{sniKey, sec.SNI}, yamlField{"client-fingerprint", sec.Fingerprint})
		if len(sec.ALPN) > 0 {
			p = append(p, yamlField{"alpn", sec.ALPN})
		}
		if sec.Type == SecurityReality {
			ro := yamlNode{{"public-key", sec.RealityPublicKey}}
			if len(sec.RealityShortIDs) > 0 {
				ro = append(ro, yamlField{"short-id", sec.RealityShortIDs[0]})
			}
			p = append(p, yamlField{"reality-opts", ro})
		}
		if e.selfSigned() {
			p = append(p, yamlField{"skip-cert-verify", true})
		}
	}
	return p, true
}

// SingboxOutbound renders a sing-box outbound (1.11+ schema).
func SingboxOutbound(e Endpoint) (map[string]interface{}, bool) {
	in := e.Inbound
	if in.Transport.Type == TransportXHTTP {
		return nil, false
	}
	o := map[string]interface{}{
		"tag":         e.Name,
		"server":      strings.Trim(e.Host, "[]"),
		"server_port": e.Port,
	}
	switch in.Protocol {
	case ProtocolVLESS:
		o["type"] = "vless"
		o["uuid"] = in.Settings.UUID
		if in.Settings.Flow != "" {
			o["flow"] = in.Settings.Flow
		}
	case ProtocolVMess:
		o["type"] = "vmess"
		o["uuid"] = in.Settings.UUID
		o["security"] = "auto"
	case ProtocolTrojan:
		o["type"] = "trojan"
		o["password"] = in.Settings.Password
	case ProtocolShadowsocks:
		o["type"] = "shadowsocks"
		o["method"] = in.Settings.Method
		o["password"] = in.Settings.Password
		return o, true
	default:
		return nil, false
	}

	switch in.Transport.Type {
	case TransportWS:
		t := map[string]interface{}{"type": "ws", "path": in.Transport.Path}
		if in.Transport.Host != "" {
			t["headers"] = map[string]interface{}{"Host": in.Transport.Host}
		}
		o["transport"] = t
	case TransportGRPC:
		o["transport"] = map[string]interface{}{"type": "grpc", "service_name": in.Transport.ServiceName}
	}

	sec := in.Security
	if sec.Type != SecurityNone {
		tls := map[string]interface{}{
			"enabled":     true,
			"server_name": sec.SNI,
			"utls":        map[string]interface{}{"enabled": true, "fingerprint": sec.Fingerprint},
		}
		if len(sec.ALPN) > 0 {
			tls["alpn"] = sec.ALPN
		}
		if sec.Type == SecurityReality {
			r := map[string]interface{}{"enabled": true, "public_key": sec.RealityPublicKey}
			if len(sec.RealityShortIDs) > 0 {
				r["short_id"] = sec.RealityShortIDs[0]
			}
			tls["reality"] = r
		}
		if e.selfSigned() {
			tls["insecure"] = true
		}
		o["tls"] = tls
	}
	return o, true
}

// SurgeLine renders a Surge [Proxy] line. Surge has no VLESS, Reality,
// gRPC or XHTTP support, so those endpoints are skipped.
func SurgeLine(e Endpoint) (string, bool) {
	in := e.Inbound
	if in.Security.Type == SecurityReality || in.Transport.Type == TransportGRPC || in.Transport.Type == TransportXHTTP {
		return "", false
	}
	name := strings.NewReplacer(",", " ", "=", " ", "\n", " ").Replace(e.Name)
	host := strings.Trim(e.Host, "[]")
	parts := []string{}
	switch in.Protocol {
	case ProtocolShadowsocks:
		parts = append(parts, "ss", host, strconv.Itoa(e.Port),
			"encrypt-method="+in.Settings.Method, "password="+in.Settings.Password, "udp-relay=true")
		return name + " = " + strings.Join(parts, ", "), true
	case ProtocolVMess:
		parts = append(parts, "vmess", host, strconv.Itoa(e.Port), "username="+in.Settings.UUID, "vmess-aead=true")
	case ProtocolTrojan:
		parts = append(parts, "trojan", host, strconv.Itoa(e.Port), "password="+in.Settings.Password)
	default:
		return "", false
	}
	if in.Transport.Type == TransportWS {
		parts = append(parts, "ws=true", "ws-path="+in.Transport.Path)
		if in.Transport.Host != "" {
			parts = append(parts, "ws-headers=Host:"+in.Transport.Host)
		}
	}
	if in.Security.Type == SecurityTLS {
		if in.Protocol == ProtocolVMess {
			parts = append(parts, "tls=true")
		}
		parts = append(parts, "sni="+in.Security.SNI)
		if e.selfSigned() {
			parts = append(parts, "skip-cert-verify=true")
		}
	}
	return name + " = " + strings.Join(parts, ", "), true
}

// FormatForUserAgent picks a subscription format from the client's UA.
func FormatForUserAgent(ua string) string {
	ua = strings.ToLower(ua)
	switch {
	case strings.Contains(ua, "sing-box") || strings.Contains(ua, "singbox") || strings.Contains(ua, "sfa") || strings.Contains(ua, "sfi") || strings.Contains(ua, "sfm"):
		return FormatSingbox
	case strings.Contains(ua, "clash") || strings.Contains(ua, "mihomo") || strings.Contains(ua, "stash") || strings.Contains(ua, "meta"):
		return FormatClash
	case strings.Contains(ua, "surge"):
		return FormatSurge
	}
	return FormatBase64
}

// Subscription renders every supported endpoint in the requested format and
// returns the body with its content type.
func Subscription(format string, endpoints []Endpoint) (string, string, error) {
	endpoints = uniqueNames(endpoints)
	switch format {
	case FormatBase64:
		lines := make([]string, 0, len(endpoints))
		for _, e := range endpoints {
			if uri, ok := ShareURI(e); ok {
				lines = append(lines, uri)
			}
		}
		return base64.StdEncoding.EncodeToString([]byte(strings.Join(lines, "\n"))), "text/plain; charset=utf-8", nil
	case FormatClash:
		return clashProfile(endpoints), "text/yaml; charset=utf-8", nil
	case FormatSingbox:
		body, err := singboxProfile(endpoints)
		return body, "application/json; charset=utf-8", err
	case FormatSurge:
		lines := []string{"[Proxy]"}
		names := []string{}
		for _, e := range endpoints {
			if line, ok := SurgeLine(e); ok {
				lines = append(lines, line)
				names = append(names, strings.SplitN(line, " = ", 2)[0])
			}
		}
		lines = append(lines, "", "[Proxy Group]")
		if len(names) > 0 {
			lines = append(lines, "节点选择 = select, "+strings.Join(names, ", "))
		} else {
			lines = append(lines, "节点选择 = select, DIRECT")
		}
		lines = append(lines, "", "[Rule]", "GEOIP,LAN,DIRECT", "FINAL,节点选择", "")
		return strings.Join(lines, "\n"), "text/plain; charset=utf-8", nil
	}
	return "", "", fmt.Errorf("不支持的订阅格式 %s", format)
}

func clashProfile(endpoints []Endpoint) string {
	proxies := []interface{}{}
	names := []interface{}{}
	for _, e := range endpoints {
		if p, ok := ClashProxy(e); ok {
			proxies = append(proxies, p)
			names = append(names, e.Name)
		}
	}
	selectList := append([]interface{}{"自动选择"}, names...)
	autoList := names
	if len(names) == 0 {
		selectList = []interface{}{"DIRECT"}
		autoList = []interface{}{"DIRECT"}
	}
	profile := yamlNode{
		{"mixed-port", 7890},
		{"allow-lan", false},
		{"mode", "rule"},
		{"log-level", "warning"},
		{"proxies", proxies},
		{"proxy-groups", []interface{}{
			yamlNode{{"name", "节点选择"}, {"type", "select"}, {"proxies", selectList}},
			yamlNode{{"name", "自动选择"}, {"type", "url-test"}, {"url", "https://www.gstatic.com/generate_204"},
				{"interval", 300}, {"proxies", autoList}},
		}},
		{"rules", []interface{}{"GEOIP,LAN,DIRECT,no-resolve", "MATCH,节点选择"}},
	}
	return marshalYAML(profile)
}

func singboxProfile(endpoints []Endpoint) (string, error) {
	outbounds := []interface{}{}
	names := []interface{}{}
	for _, e := range endpoints {
		if o, ok := SingboxOutbound(e); ok {
			outbounds = append(outbounds, o)
			names = append(names, e.Name)
		}
	}
	selectList := append([]interface{}{"自动选择"}, names...)
	autoList := names
	if len(names) == 0 {
		selectList = []interface{}{"direct"}
		autoList = []interface{}{"direct"}
	}
	groups := []interface{}{
		map[string]interface{}{"type": "selector", "tag": "节点选择", "outbounds": selectList},
		map[string]interface{}{"type": "urltest", "tag": "自动选择", "outbounds": autoList,
			"url": "https://www.gstatic.com/generate_204", "interval": "5m"},
	}
	cfg := map[string]interface{}{
		"log": map[string]interface{}{"level": "warn"},
		"inbounds": []interface{}{
			map[string]interface{}{"type": "tun", "tag": "tun-in", "address": []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"},
				"auto_route": true, "strict_route": true},
			map[string]interface{}{"type": "mixed", "tag": "mixed-in", "listen": "127.0.0.1", "listen_port": 2080},
		},
		"outbounds": append(append(groups, outbounds...), map[string]interface{}{"type": "direct", "tag": "direct"}),
		"route": map[string]interface{}{
			"auto_detect_interface": true,
			"final":                 "节点选择",
			"rules": []interface{}{
				map[string]interface{}{"action": "sniff"},
				map[string]interface{}{"protocol": "dns", "action": "hijack-dns"},
				map[string]interface{}{"ip_is_private": true, "outbound": "direct"},
			},
		},
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}

// uniqueNames suffixes duplicate endpoint names; Clash proxy names and
// sing-box tags must be unique or the whole profile is rejected.
func uniqueNames(endpoints []Endpoint) []Endpoint {
	out := make([]Endpoint, len(endpoints))
	seen := map[string]bool{"节点选择": true, "自动选择": true, "direct": true, "DIRECT": true, "REJECT": true}
	for i, e := range endpoints {
		base := strings.TrimSpace(e.Name)
		if base == "" {
			base = fmt.Sprintf("node-%d", i+1)
		}
		name := base
		for n := 2; seen[name]; n++ {
			name = fmt.Sprintf("%s #%d", base, n)
		}
		seen[name] = true
		e.Name = name
		out[i] = e
	}
	return out
}
