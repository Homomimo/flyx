// Package kernel turns panel-side inbound definitions into proxy-kernel
// configuration (Xray, sing-box, Snell, ...). Everything here is pure so it can
// be covered by golden tests; the HTTP layer only persists and dispatches.
package kernel

import "strings"

const (
	TypeXray    = "xray"
	TypeSingbox = "singbox"
	TypeSnell   = "snell"
)

const (
	ProtocolVLESS       = "vless"
	ProtocolVMess       = "vmess"
	ProtocolTrojan      = "trojan"
	ProtocolShadowsocks = "shadowsocks"
)

const (
	TransportTCP   = "tcp"
	TransportWS    = "ws"
	TransportGRPC  = "grpc"
	TransportXHTTP = "xhttp"
)

const (
	SecurityNone    = "none"
	SecurityTLS     = "tls"
	SecurityReality = "reality"
)

// TransportCap lists which security layers a transport accepts for a protocol.
type TransportCap struct {
	Type       string   `json:"type"`
	Securities []string `json:"securities"`
}

type ProtocolCap struct {
	Protocol   string         `json:"protocol"`
	Transports []TransportCap `json:"transports"`
}

// Download describes where an agent fetches the kernel binary. {VERSION} and
// {ARCH} are substituted by the agent; ArchMap translates GOARCH to the
// release asset naming of the kernel project.
type Download struct {
	URL         string            `json:"url"`
	LatestURL   string            `json:"latestUrl"`
	ChecksumURL string            `json:"checksumUrl"`
	LatestSum   string            `json:"latestChecksumUrl"`
	Archive     string            `json:"archive"`
	BinName     string            `json:"binName"`
	ArchMap     map[string]string `json:"archMap"`
}

type Info struct {
	Type        string        `json:"type"`
	Name        string        `json:"name"`
	Implemented bool          `json:"implemented"`
	Protocols   []ProtocolCap `json:"protocols"`
	Download    *Download     `json:"-"`
}

var xrayDownload = &Download{
	URL:         "https://github.com/XTLS/Xray-core/releases/download/v{VERSION}/Xray-linux-{ARCH}.zip",
	LatestURL:   "https://github.com/XTLS/Xray-core/releases/latest/download/Xray-linux-{ARCH}.zip",
	ChecksumURL: "https://github.com/XTLS/Xray-core/releases/download/v{VERSION}/Xray-linux-{ARCH}.zip.dgst",
	LatestSum:   "https://github.com/XTLS/Xray-core/releases/latest/download/Xray-linux-{ARCH}.zip.dgst",
	Archive:     "zip",
	BinName:     "xray",
	ArchMap:     map[string]string{"amd64": "64", "arm64": "arm64-v8a", "386": "32", "arm": "arm32-v7a"},
}

var catalog = []Info{
	{
		Type:        TypeXray,
		Name:        "Xray",
		Implemented: true,
		Download:    xrayDownload,
		Protocols: []ProtocolCap{
			{Protocol: ProtocolVLESS, Transports: []TransportCap{
				{Type: TransportTCP, Securities: []string{SecurityReality, SecurityTLS, SecurityNone}},
				{Type: TransportXHTTP, Securities: []string{SecurityReality, SecurityTLS, SecurityNone}},
				{Type: TransportGRPC, Securities: []string{SecurityReality, SecurityTLS, SecurityNone}},
				{Type: TransportWS, Securities: []string{SecurityTLS, SecurityNone}},
			}},
			{Protocol: ProtocolVMess, Transports: []TransportCap{
				{Type: TransportTCP, Securities: []string{SecurityTLS, SecurityNone}},
				{Type: TransportWS, Securities: []string{SecurityTLS, SecurityNone}},
				{Type: TransportGRPC, Securities: []string{SecurityTLS, SecurityNone}},
			}},
			{Protocol: ProtocolTrojan, Transports: []TransportCap{
				{Type: TransportTCP, Securities: []string{SecurityTLS, SecurityReality}},
				{Type: TransportWS, Securities: []string{SecurityTLS}},
				{Type: TransportGRPC, Securities: []string{SecurityTLS, SecurityReality}},
			}},
			{Protocol: ProtocolShadowsocks, Transports: []TransportCap{
				{Type: TransportTCP, Securities: []string{SecurityNone}},
			}},
		},
	},
	{Type: TypeSingbox, Name: "sing-box"},
	{Type: TypeSnell, Name: "Snell"},
}

// Catalog returns every known kernel; unimplemented ones are listed so the UI
// can show them as upcoming.
func Catalog() []Info {
	out := make([]Info, len(catalog))
	copy(out, catalog)
	return out
}

func Lookup(kernelType string) (Info, bool) {
	kernelType = strings.ToLower(strings.TrimSpace(kernelType))
	for _, info := range catalog {
		if info.Type == kernelType {
			return info, true
		}
	}
	return Info{}, false
}

func (info Info) protocol(name string) (ProtocolCap, bool) {
	for _, p := range info.Protocols {
		if p.Protocol == name {
			return p, true
		}
	}
	return ProtocolCap{}, false
}

func (p ProtocolCap) transport(name string) (TransportCap, bool) {
	for _, t := range p.Transports {
		if t.Type == name {
			return t, true
		}
	}
	return TransportCap{}, false
}

func containsString(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

// ResolveDownload builds the agent download spec for a version ("" or
// "latest" selects the newest release). proxy, when non-empty, is prefixed to
// every GitHub URL the same way agent upgrades are mirrored.
func ResolveDownload(info Info, version, proxy string) (map[string]interface{}, bool) {
	if info.Download == nil {
		return nil, false
	}
	d := info.Download
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	url, sum := d.LatestURL, d.LatestSum
	if version != "" && !strings.EqualFold(version, "latest") {
		url = strings.ReplaceAll(d.URL, "{VERSION}", version)
		sum = strings.ReplaceAll(d.ChecksumURL, "{VERSION}", version)
	}
	if proxy = strings.TrimSuffix(strings.TrimSpace(proxy), "/"); proxy != "" {
		url = proxy + "/" + url
		sum = proxy + "/" + sum
	}
	archMap := make(map[string]string, len(d.ArchMap))
	for k, v := range d.ArchMap {
		archMap[k] = v
	}
	return map[string]interface{}{
		"url":         url,
		"checksumUrl": sum,
		"archive":     d.Archive,
		"binName":     d.BinName,
		"archMap":     archMap,
	}, true
}
