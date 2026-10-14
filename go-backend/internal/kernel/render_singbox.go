package kernel

import (
	"fmt"
	"net"
	"strconv"
)

func renderSingbox(inbounds []Inbound) (string, error) {
	list := make([]interface{}, 0, len(inbounds))
	for _, in := range inbounds {
		items, err := singboxInbounds(in)
		if err != nil {
			return "", fmt.Errorf("入站 %d: %v", in.ID, err)
		}
		list = append(list, items...)
	}
	cfg := obj{
		"log":       obj{"level": "warn"},
		"inbounds":  list,
		"outbounds": []interface{}{obj{"type": "direct", "tag": "direct"}},
	}
	return marshalConfig(cfg)
}

// singboxInbounds returns the inbound objects for one panel inbound. A
// ShadowTLS-fronted Shadowsocks becomes two: the public shadowtls inbound
// and a loopback shadowsocks backend it detours to.
func singboxInbounds(in Inbound) ([]interface{}, error) {
	item := obj{
		"tag":         in.Tag(),
		"listen":      in.Listen,
		"listen_port": in.Port,
	}
	s := in.Settings
	switch in.Protocol {
	case ProtocolVLESS:
		user := obj{"uuid": s.UUID}
		if s.Flow != "" {
			user["flow"] = s.Flow
		}
		item["type"] = "vless"
		item["users"] = []interface{}{user}
	case ProtocolVMess:
		item["type"] = "vmess"
		item["users"] = []interface{}{obj{"uuid": s.UUID, "alterId": 0}}
	case ProtocolTrojan:
		item["type"] = "trojan"
		item["users"] = []interface{}{obj{"password": s.Password}}
	case ProtocolShadowsocks:
		if in.Security.Type == SecurityShadowTLS {
			backendTag := in.Tag() + "-ss"
			front := obj{
				"type":        "shadowtls",
				"tag":         in.Tag(),
				"listen":      in.Listen,
				"listen_port": in.Port,
				"version":     3,
				"users":       []interface{}{obj{"password": in.Security.ShadowTLSPassword}},
				"handshake":   obj{"server": in.Security.SNI, "server_port": 443},
				"detour":      backendTag,
			}
			backend := obj{
				"type":     "shadowsocks",
				"tag":      backendTag,
				"listen":   "127.0.0.1",
				"method":   s.Method,
				"password": s.Password,
			}
			if err := mergeExtra(front, in.Extra); err != nil {
				return nil, err
			}
			return []interface{}{front, backend}, nil
		}
		item["type"] = "shadowsocks"
		item["method"] = s.Method
		item["password"] = s.Password
	case ProtocolHysteria2:
		item["type"] = "hysteria2"
		item["users"] = []interface{}{obj{"password": s.Password}}
		item["ignore_client_bandwidth"] = true
		if s.Masquerade != "" {
			item["masquerade"] = s.Masquerade
		}
	case ProtocolTUIC:
		item["type"] = "tuic"
		item["users"] = []interface{}{obj{"uuid": s.UUID, "password": s.Password}}
		item["congestion_control"] = "bbr"
	case ProtocolAnyTLS:
		item["type"] = "anytls"
		item["users"] = []interface{}{obj{"password": s.Password}}
	default:
		return nil, fmt.Errorf("sing-box 不支持协议 %s", in.Protocol)
	}

	if tls, err := singboxTLS(in.Security); err != nil {
		return nil, err
	} else if tls != nil {
		item["tls"] = tls
	}
	switch in.Transport.Type {
	case TransportWS:
		t := obj{"type": "ws", "path": in.Transport.Path}
		if in.Transport.Host != "" {
			t["headers"] = obj{"Host": in.Transport.Host}
		}
		item["transport"] = t
	case TransportGRPC:
		item["transport"] = obj{"type": "grpc", "service_name": in.Transport.ServiceName}
	}
	if err := mergeExtra(item, in.Extra); err != nil {
		return nil, err
	}
	return []interface{}{item}, nil
}

func singboxTLS(sec Security) (obj, error) {
	switch sec.Type {
	case SecurityTLS:
		tls := obj{"enabled": true, "server_name": sec.SNI}
		if len(sec.ALPN) > 0 {
			tls["alpn"] = sec.ALPN
		}
		if sec.CertMode == "file" {
			tls["certificate_path"] = sec.CertFile
			tls["key_path"] = sec.KeyFile
		} else {
			tls["certificate"] = pemLines(sec.CertPEM)
			tls["key"] = pemLines(sec.KeyPEM)
		}
		return tls, nil
	case SecurityReality:
		host, portStr, err := net.SplitHostPort(sec.RealityDest)
		if err != nil {
			return nil, fmt.Errorf("Reality 目标地址错误")
		}
		port, _ := strconv.Atoi(portStr)
		return obj{
			"enabled":     true,
			"server_name": sec.SNI,
			"reality": obj{
				"enabled":     true,
				"handshake":   obj{"server": host, "server_port": port},
				"private_key": sec.RealityPrivateKey,
				"short_id":    sec.RealityShortIDs,
			},
		}, nil
	}
	return nil, nil
}
