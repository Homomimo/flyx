// Package kernel runs third-party proxy kernels (Xray, sing-box, Snell, ...)
// next to the agent. The panel renders every configuration file; this package
// only installs binaries, writes files, supervises processes and counts
// per-inbound traffic with nftables.
package kernel

import (
	"fmt"
	"net"
	"path/filepath"
	"regexp"
	"strings"
)

type Download struct {
	URL         string            `json:"url"`
	ChecksumURL string            `json:"checksumUrl"`
	Archive     string            `json:"archive"` // zip | tar.gz | raw
	BinName     string            `json:"binName"`
	ArchMap     map[string]string `json:"archMap"`
}

type File struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type Port struct {
	InboundID int64  `json:"inboundId"`
	Port      int    `json:"port"`
	Network   string `json:"network"` // "tcp", "udp" or "tcp,udp"
	// Allow, when present, restricts the port to these source CIDRs; every
	// other packet to it is dropped. Absent means no restriction.
	Allow []string `json:"allow,omitempty"`
}

// ApplyRequest is the KernelApply command body.
type ApplyRequest struct {
	Type     string    `json:"type"`
	Version  string    `json:"version"`
	Download *Download `json:"download"`
	Files    []File    `json:"files"`
	Args     []string  `json:"args"`
	Hash     string    `json:"hash"`
	Enabled  bool      `json:"enabled"`
	Ports    []Port    `json:"ports"`
}

type State struct {
	Type       string   `json:"type"`
	Running    bool     `json:"running"`
	Enabled    bool     `json:"enabled"`
	Pid        int      `json:"pid"`
	Version    string   `json:"version"`
	ConfigHash string   `json:"configHash"`
	Error      string   `json:"error"`
	Restarts   int      `json:"restarts"`
	StartedAt  int64    `json:"startedAt"`
	Logs       []string `json:"logs,omitempty"`
}

type Counter struct {
	InboundID int64  `json:"inboundId"`
	Proto     string `json:"proto"`
	Dir       string `json:"dir"` // "in" = client upload, "out" = client download
	Bytes     int64  `json:"bytes"`
}

type Status struct {
	Kernels      []State   `json:"kernels"`
	Counters     []Counter `json:"counters"`
	Epoch        string    `json:"epoch"`
	NftAvailable bool      `json:"nftAvailable"`
	NftError     string    `json:"nftError,omitempty"`
}

type adapter struct {
	versionArgs []string
}

var adapters = map[string]adapter{
	"xray":    {versionArgs: []string{"version"}},
	"singbox": {versionArgs: []string{"version"}},
	"snell":   {versionArgs: []string{"-v"}},
}

var (
	safeNamePattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	safeVersionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,39}$`)
)

func safeName(name string) bool {
	return safeNamePattern.MatchString(name) && !strings.Contains(name, "..")
}

func (r *ApplyRequest) validate() error {
	if _, ok := adapters[r.Type]; !ok {
		return fmt.Errorf("不支持的内核类型: %s", r.Type)
	}
	if r.Version == "" {
		r.Version = "latest"
	}
	if !safeVersionPattern.MatchString(r.Version) || strings.Contains(r.Version, "..") {
		return fmt.Errorf("版本号格式错误: %s", r.Version)
	}
	if r.Download == nil || !safeName(r.Download.BinName) {
		return fmt.Errorf("缺少下载信息")
	}
	switch r.Download.Archive {
	case "zip", "tar.gz", "raw":
	default:
		return fmt.Errorf("不支持的压缩格式: %s", r.Download.Archive)
	}
	for _, f := range r.Files {
		if !safeName(f.Name) || filepath.Base(f.Name) != f.Name {
			return fmt.Errorf("非法配置文件名: %s", f.Name)
		}
	}
	for _, p := range r.Ports {
		if p.Port < 1 || p.Port > 65535 || p.InboundID <= 0 {
			return fmt.Errorf("非法端口: %d", p.Port)
		}
		if len(p.Allow) > 512 {
			return fmt.Errorf("白名单条目过多")
		}
		for _, c := range p.Allow {
			if _, _, err := net.ParseCIDR(c); err != nil {
				return fmt.Errorf("非法白名单条目: %q", c)
			}
		}
	}
	return nil
}
