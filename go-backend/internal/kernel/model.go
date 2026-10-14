package kernel

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

type Settings struct {
	UUID     string `json:"uuid,omitempty"`
	Flow     string `json:"flow,omitempty"`
	Password string `json:"password,omitempty"`
	Method   string `json:"method,omitempty"`
}

type Transport struct {
	Type        string `json:"type"`
	Path        string `json:"path,omitempty"`
	Host        string `json:"host,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
	Mode        string `json:"mode,omitempty"`
}

type Security struct {
	Type        string   `json:"type"`
	SNI         string   `json:"sni,omitempty"`
	ALPN        []string `json:"alpn,omitempty"`
	Fingerprint string   `json:"fingerprint,omitempty"`

	// TLS: "self" stores a generated certificate inline, "file" points at
	// paths that already exist on the node.
	CertMode string `json:"certMode,omitempty"`
	CertPEM  string `json:"certPem,omitempty"`
	KeyPEM   string `json:"keyPem,omitempty"`
	CertFile string `json:"certFile,omitempty"`
	KeyFile  string `json:"keyFile,omitempty"`

	RealityDest       string   `json:"realityDest,omitempty"`
	RealityPrivateKey string   `json:"realityPrivateKey,omitempty"`
	RealityPublicKey  string   `json:"realityPublicKey,omitempty"`
	RealityShortIDs   []string `json:"realityShortIds,omitempty"`
}

// Inbound is the kernel-independent description of one listening service.
type Inbound struct {
	ID        int64           `json:"id"`
	Protocol  string          `json:"protocol"`
	Listen    string          `json:"listen"`
	Port      int             `json:"port"`
	Settings  Settings        `json:"settings"`
	Transport Transport       `json:"transport"`
	Security  Security        `json:"security"`
	Extra     json.RawMessage `json:"extra,omitempty"`
}

func (in Inbound) Tag() string {
	return fmt.Sprintf("in-%d", in.ID)
}

// Network reports which L4 protocols the inbound listens on, used for nft
// traffic counters.
func (in Inbound) Network() string {
	if in.Protocol == ProtocolShadowsocks {
		return "tcp,udp"
	}
	return "tcp"
}

const (
	FlowVision          = "xtls-rprx-vision"
	DefaultRealitySNI   = "www.microsoft.com"
	DefaultSSMethod     = "2022-blake3-aes-128-gcm"
	DefaultFingerprint  = "chrome"
	DefaultTLSCertMode  = "self"
	defaultGRPCService  = "grpc"
	defaultXHTTPMode    = "auto"
	maxExtraConfigBytes = 64 * 1024
)

var ssMethods = []string{
	"2022-blake3-aes-128-gcm",
	"2022-blake3-aes-256-gcm",
	"2022-blake3-chacha20-poly1305",
	"aes-128-gcm",
	"aes-256-gcm",
	"chacha20-ietf-poly1305",
}

var (
	shortIDPattern = regexp.MustCompile(`^[0-9a-f]{0,16}$`)
	hostPattern    = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)
)

// SSMethods lists the Shadowsocks ciphers the panel offers.
func SSMethods() []string {
	out := make([]string, len(ssMethods))
	copy(out, ssMethods)
	return out
}

// Normalize validates an inbound against a kernel's capabilities, fills
// defaults and generates any missing credentials. It mutates in place so the
// caller persists exactly what will be rendered.
func Normalize(kernelType string, in *Inbound) error {
	if in == nil {
		return errors.New("入站不能为空")
	}
	info, ok := Lookup(kernelType)
	if !ok || !info.Implemented {
		return fmt.Errorf("暂不支持内核 %s", kernelType)
	}

	in.Protocol = strings.ToLower(strings.TrimSpace(in.Protocol))
	proto, ok := info.protocol(in.Protocol)
	if !ok {
		return fmt.Errorf("%s 不支持协议 %s", info.Name, in.Protocol)
	}

	in.Transport.Type = strings.ToLower(strings.TrimSpace(in.Transport.Type))
	if in.Transport.Type == "" {
		in.Transport.Type = TransportTCP
	}
	tcap, ok := proto.transport(in.Transport.Type)
	if !ok {
		return fmt.Errorf("%s 协议不支持传输方式 %s", in.Protocol, in.Transport.Type)
	}

	in.Security.Type = strings.ToLower(strings.TrimSpace(in.Security.Type))
	if in.Security.Type == "" {
		in.Security.Type = tcap.Securities[0]
	}
	if !containsString(tcap.Securities, in.Security.Type) {
		return fmt.Errorf("%s + %s 不支持安全层 %s", in.Protocol, in.Transport.Type, in.Security.Type)
	}

	if in.Port < 1 || in.Port > 65535 {
		return errors.New("端口必须在 1-65535 之间")
	}
	in.Listen = strings.TrimSpace(in.Listen)
	if in.Listen == "" {
		in.Listen = "::"
	}
	if net.ParseIP(in.Listen) == nil {
		return errors.New("监听地址必须是 IP")
	}

	if err := normalizeExtra(in); err != nil {
		return err
	}
	if err := normalizeTransport(&in.Transport); err != nil {
		return err
	}
	if err := normalizeSettings(in); err != nil {
		return err
	}
	return normalizeSecurity(&in.Security)
}

func normalizeExtra(in *Inbound) error {
	raw := strings.TrimSpace(string(in.Extra))
	if raw == "" || raw == "null" || raw == "{}" {
		in.Extra = nil
		return nil
	}
	if len(raw) > maxExtraConfigBytes {
		return errors.New("高级配置过大")
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return errors.New("高级配置必须是 JSON 对象")
	}
	in.Extra = json.RawMessage(raw)
	return nil
}

func normalizeTransport(t *Transport) error {
	t.Path = strings.TrimSpace(t.Path)
	t.Host = strings.TrimSpace(t.Host)
	t.ServiceName = strings.TrimSpace(t.ServiceName)
	t.Mode = strings.TrimSpace(t.Mode)
	if t.Host != "" && !hostPattern.MatchString(t.Host) {
		return errors.New("Host 格式错误")
	}
	switch t.Type {
	case TransportTCP:
		t.Path, t.Host, t.ServiceName, t.Mode = "", "", "", ""
	case TransportWS, TransportXHTTP:
		if t.Path == "" {
			t.Path = "/" + RealityShortID()[:8]
		}
		if !strings.HasPrefix(t.Path, "/") {
			t.Path = "/" + t.Path
		}
		t.ServiceName = ""
		if t.Type == TransportWS {
			t.Mode = ""
		} else if t.Mode == "" {
			t.Mode = defaultXHTTPMode
		}
	case TransportGRPC:
		if t.ServiceName == "" {
			t.ServiceName = defaultGRPCService
		}
		t.Path, t.Mode = "", ""
	}
	return nil
}

func normalizeSettings(in *Inbound) error {
	s := &in.Settings
	s.UUID = strings.TrimSpace(s.UUID)
	s.Password = strings.TrimSpace(s.Password)
	s.Method = strings.TrimSpace(s.Method)
	s.Flow = strings.TrimSpace(s.Flow)

	switch in.Protocol {
	case ProtocolVLESS, ProtocolVMess:
		if s.UUID == "" {
			s.UUID = NewUUID()
		}
		if !uuidPattern.MatchString(strings.ToLower(s.UUID)) {
			return errors.New("UUID 格式错误")
		}
		s.Password, s.Method = "", ""
		visionOK := in.Protocol == ProtocolVLESS && in.Transport.Type == TransportTCP && in.Security.Type != SecurityNone
		if s.Flow != "" && (s.Flow != FlowVision || !visionOK) {
			s.Flow = ""
		}
	case ProtocolTrojan:
		if s.Password == "" {
			s.Password = RandomPassword()
		}
		s.UUID, s.Method, s.Flow = "", "", ""
	case ProtocolShadowsocks:
		if s.Method == "" {
			s.Method = DefaultSSMethod
		}
		if !containsString(ssMethods, s.Method) {
			return fmt.Errorf("不支持的加密方式 %s", s.Method)
		}
		if s.Password == "" {
			s.Password = ShadowsocksPassword(s.Method)
		}
		if n := SS2022KeyLen(s.Method); n > 0 {
			key, err := base64.StdEncoding.DecodeString(s.Password)
			if err != nil || len(key) != n {
				return fmt.Errorf("%s 的密钥必须是 %d 字节的 base64", s.Method, n)
			}
		}
		s.UUID, s.Flow = "", ""
	}
	return nil
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func normalizeSecurity(sec *Security) error {
	sec.SNI = strings.TrimSpace(sec.SNI)
	sec.Fingerprint = strings.TrimSpace(sec.Fingerprint)
	if sec.SNI != "" && !hostPattern.MatchString(sec.SNI) {
		return errors.New("SNI 格式错误")
	}

	switch sec.Type {
	case SecurityNone:
		*sec = Security{Type: SecurityNone}
		return nil
	case SecurityTLS:
		sec.RealityDest, sec.RealityPrivateKey, sec.RealityPublicKey, sec.RealityShortIDs = "", "", "", nil
		if sec.Fingerprint == "" {
			sec.Fingerprint = DefaultFingerprint
		}
		if sec.CertMode == "" {
			sec.CertMode = DefaultTLSCertMode
		}
		switch sec.CertMode {
		case "self":
			if sec.SNI == "" {
				return errors.New("TLS 需要填写域名 (SNI)")
			}
			sec.CertFile, sec.KeyFile = "", ""
			if sec.CertPEM == "" || sec.KeyPEM == "" || !certMatchesName(sec.CertPEM, sec.SNI) {
				cert, key, err := SelfSignedCert(sec.SNI)
				if err != nil {
					return fmt.Errorf("生成自签证书失败: %v", err)
				}
				sec.CertPEM, sec.KeyPEM = cert, key
			}
		case "file":
			sec.CertPEM, sec.KeyPEM = "", ""
			sec.CertFile = strings.TrimSpace(sec.CertFile)
			sec.KeyFile = strings.TrimSpace(sec.KeyFile)
			if !strings.HasPrefix(sec.CertFile, "/") || !strings.HasPrefix(sec.KeyFile, "/") {
				return errors.New("证书和私钥需要填写节点上的绝对路径")
			}
		default:
			return fmt.Errorf("未知证书模式 %s", sec.CertMode)
		}
		return nil
	case SecurityReality:
		sec.CertMode, sec.CertPEM, sec.KeyPEM, sec.CertFile, sec.KeyFile = "", "", "", "", ""
		if sec.SNI == "" {
			sec.SNI = DefaultRealitySNI
		}
		if sec.Fingerprint == "" {
			sec.Fingerprint = DefaultFingerprint
		}
		sec.RealityDest = strings.TrimSpace(sec.RealityDest)
		if sec.RealityDest == "" {
			sec.RealityDest = sec.SNI + ":443"
		}
		if host, port, err := net.SplitHostPort(sec.RealityDest); err != nil || host == "" {
			return errors.New("Reality 目标地址格式应为 host:port")
		} else if p, err := strconv.Atoi(port); err != nil || p < 1 || p > 65535 {
			return errors.New("Reality 目标端口错误")
		}
		sec.RealityPrivateKey = strings.TrimSpace(sec.RealityPrivateKey)
		if sec.RealityPrivateKey == "" {
			priv, pub, err := RealityKeyPair()
			if err != nil {
				return fmt.Errorf("生成 Reality 密钥失败: %v", err)
			}
			sec.RealityPrivateKey, sec.RealityPublicKey = priv, pub
		} else {
			pub, err := RealityPublicKey(sec.RealityPrivateKey)
			if err != nil {
				return err
			}
			sec.RealityPublicKey = pub
		}
		ids := make([]string, 0, len(sec.RealityShortIDs))
		for _, id := range sec.RealityShortIDs {
			id = strings.ToLower(strings.TrimSpace(id))
			if !shortIDPattern.MatchString(id) || len(id)%2 != 0 {
				return errors.New("Reality shortId 必须是偶数位十六进制（最多 16 位）")
			}
			ids = append(ids, id)
		}
		if len(ids) == 0 {
			ids = []string{RealityShortID()}
		}
		sec.RealityShortIDs = ids
		return nil
	}
	return fmt.Errorf("未知安全层 %s", sec.Type)
}

// Regenerate replaces every secret of an inbound while keeping its shape.
func Regenerate(kernelType string, in *Inbound) error {
	in.Settings.UUID = ""
	in.Settings.Password = ""
	in.Security.RealityPrivateKey = ""
	in.Security.RealityPublicKey = ""
	in.Security.RealityShortIDs = nil
	in.Security.CertPEM = ""
	in.Security.KeyPEM = ""
	return Normalize(kernelType, in)
}
