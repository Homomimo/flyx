package kernel

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const nftTable = "flvx_kernel"

// nftRunner executes `nft` with a script on stdin (or arguments). It is a
// variable so tests can capture scripts without root.
var nftRunner = func(ctx context.Context, stdin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "nft", args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return out, fmt.Errorf("nft: %s", msg)
	}
	return out, nil
}

var nftAvailable = func() error {
	_, err := exec.LookPath("nft")
	return err
}

type counterRule struct {
	InboundID int64
	Port      int
	Proto     string
}

func (c counterRule) name(dir string) string {
	return fmt.Sprintf("%s_%d_%s", dir, c.InboundID, c.Proto)
}

// expandPorts turns inbound ports into one rule per L4 protocol, sorted so
// the generated ruleset (and its signature) is stable.
func expandPorts(ports []Port) []counterRule {
	seen := make(map[counterRule]struct{})
	var rules []counterRule
	for _, p := range ports {
		for _, proto := range strings.Split(p.Network, ",") {
			proto = strings.TrimSpace(proto)
			if proto != "tcp" && proto != "udp" {
				continue
			}
			r := counterRule{InboundID: p.InboundID, Port: p.Port, Proto: proto}
			if _, ok := seen[r]; ok {
				continue
			}
			seen[r] = struct{}{}
			rules = append(rules, r)
		}
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].InboundID != rules[j].InboundID {
			return rules[i].InboundID < rules[j].InboundID
		}
		return rules[i].Proto < rules[j].Proto
	})
	return rules
}

// nftScript builds an atomic ruleset that only counts: every chain has policy
// accept and no rule drops, so it never interferes with other firewalls.
func nftScript(rules []counterRule) string {
	var b strings.Builder
	fmt.Fprintf(&b, "table inet %s {}\n", nftTable)
	fmt.Fprintf(&b, "delete table inet %s\n", nftTable)
	if len(rules) == 0 {
		return b.String()
	}
	fmt.Fprintf(&b, "table inet %s {\n", nftTable)
	for _, r := range rules {
		fmt.Fprintf(&b, "\tcounter %s {}\n", r.name("in"))
		fmt.Fprintf(&b, "\tcounter %s {}\n", r.name("out"))
	}
	b.WriteString("\tchain input {\n\t\ttype filter hook input priority -150; policy accept;\n")
	for _, r := range rules {
		fmt.Fprintf(&b, "\t\t%s dport %d counter name %s\n", r.Proto, r.Port, r.name("in"))
	}
	b.WriteString("\t}\n\tchain output {\n\t\ttype filter hook output priority -150; policy accept;\n")
	for _, r := range rules {
		fmt.Fprintf(&b, "\t\t%s sport %d counter name %s\n", r.Proto, r.Port, r.name("out"))
	}
	b.WriteString("\t}\n}\n")
	return b.String()
}

type nftState struct {
	Signature string `json:"signature"`
	Epoch     string `json:"epoch"`
}

// nftCounters owns the flvx_kernel table. The epoch changes whenever the
// table is rebuilt so the panel knows the counters restarted from zero.
type nftCounters struct {
	mu        sync.Mutex
	statePath string
	state     nftState
	lastErr   string
}

func newNftCounters(baseDir string) *nftCounters {
	n := &nftCounters{statePath: filepath.Join(baseDir, "nft.json")}
	if b, err := os.ReadFile(n.statePath); err == nil {
		_ = json.Unmarshal(b, &n.state)
	}
	return n
}

func (n *nftCounters) tableExists(ctx context.Context) bool {
	_, err := nftRunner(ctx, "", "list", "table", "inet", nftTable)
	return err == nil
}

func (n *nftCounters) sync(ports []Port) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if err := nftAvailable(); err != nil {
		n.lastErr = "未安装 nftables，无法统计流量"
		return nil
	}
	rules := expandPorts(ports)
	script := nftScript(rules)
	sig := hashString(script)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if sig == n.state.Signature && (len(rules) == 0 || n.tableExists(ctx)) {
		n.lastErr = ""
		return nil
	}
	if _, err := nftRunner(ctx, script, "-f", "-"); err != nil {
		n.lastErr = err.Error()
		return err
	}
	n.lastErr = ""
	n.state = nftState{Signature: sig, Epoch: newEpoch()}
	if b, err := json.Marshal(n.state); err == nil {
		_ = writeFileAtomic(n.statePath, b, 0o600)
	}
	return nil
}

type nftListOutput struct {
	Nftables []struct {
		Counter *struct {
			Name  string `json:"name"`
			Table string `json:"table"`
			Bytes int64  `json:"bytes"`
		} `json:"counter"`
	} `json:"nftables"`
}

// read returns the current counters together with the epoch they belong to.
func (n *nftCounters) read() ([]Counter, string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if err := nftAvailable(); err != nil {
		return nil, "", fmt.Errorf("未安装 nftables，无法统计流量")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := nftRunner(ctx, "", "-j", "list", "counters", "table", "inet", nftTable)
	if err != nil {
		return nil, "", err
	}
	counters, err := parseCounters(out)
	return counters, n.state.Epoch, err
}

func parseCounters(raw []byte) ([]Counter, error) {
	var parsed nftListOutput
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("解析 nft 计数失败: %v", err)
	}
	var counters []Counter
	for _, item := range parsed.Nftables {
		c := item.Counter
		if c == nil || c.Table != nftTable {
			continue
		}
		parts := strings.Split(c.Name, "_")
		if len(parts) != 3 || (parts[0] != "in" && parts[0] != "out") {
			continue
		}
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		counters = append(counters, Counter{InboundID: id, Proto: parts[2], Dir: parts[0], Bytes: c.Bytes})
	}
	return counters, nil
}

func newEpoch() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
