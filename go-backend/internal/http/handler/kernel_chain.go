package handler

import (
	"errors"
	"fmt"
	"log"
	"net"
	"strings"

	"go-backend/internal/kernel"
	"go-backend/internal/store/model"
)

// Chained outbounds: an inbound with OutboundID set leaves through another
// inbound (of this panel, or an imported node) instead of from its own node.
// Each kernel dials only the next hop, so A -> B -> C is A chained to B and
// B chained to C.

// maxChainHops bounds the walk that looks for loops.
const maxChainHops = 32

// chainEndpoint is the address a kernel on fromNodeID dials to reach the
// target inbound.
func (h *Handler) chainEndpoint(target *model.NodeInbound, fromNodeID int64) (kernel.Endpoint, error) {
	in, err := inboundFromModel(*target)
	if err != nil {
		return kernel.Endpoint{}, err
	}
	// One port: a chained hop never uses Hysteria2 port hopping.
	e := kernel.Endpoint{Name: target.Name, Port: target.Port, Inbound: in, Proxy: importedProxy(target)}
	if isExternalInbound(target) {
		e.Host = firstNonEmpty(strings.TrimSpace(target.ShareHost), target.Listen)
		return e, nil
	}
	if target.AcceptProxyProtocol == 1 {
		return e, fmt.Errorf("出站节点「%s」开启了获取真实 IP（PROXY protocol），只能经中转连接", target.Name)
	}
	if target.NodeID == fromNodeID {
		// Same machine: dial it locally.
		e.Host = "127.0.0.1"
		if ip := net.ParseIP(target.Listen); ip != nil && !ip.IsUnspecified() {
			e.Host = ip.String()
		}
		return e, nil
	}
	if isLoopbackListen(target.Listen) {
		return e, fmt.Errorf("出站节点「%s」只监听本机，其他节点连不上", target.Name)
	}
	e.Host = strings.TrimSpace(target.ShareHost)
	if e.Host == "" {
		if node, err := h.repo.GetNodeByID(target.NodeID); err == nil && node != nil {
			e.Host = strings.TrimSpace(node.ServerIP)
		}
	}
	if e.Host == "" {
		return e, fmt.Errorf("出站节点「%s」没有可用的地址", target.Name)
	}
	return e, nil
}

// inboundVia resolves the chained outbound of an inbound for rendering. A
// deleted target yields an empty endpoint, which drops the traffic.
func (h *Handler) inboundVia(row model.NodeInbound) (*kernel.Endpoint, error) {
	if row.OutboundID == 0 {
		return nil, nil
	}
	target, err := h.repo.GetNodeInbound(row.OutboundID)
	if err != nil {
		return nil, err
	}
	if target == nil {
		log.Printf("kernel: inbound %d chains through missing inbound %d, dropping its traffic", row.ID, row.OutboundID)
		return &kernel.Endpoint{}, nil
	}
	e, err := h.chainEndpoint(target, row.NodeID)
	if err != nil {
		return nil, fmt.Errorf("入站「%s」的出站代理: %v", row.Name, err)
	}
	return &e, nil
}

// validateChain checks the chained outbound of an inbound about to be saved.
func (h *Handler) validateChain(row model.NodeInbound, kernelType string) error {
	if row.OutboundID == 0 {
		return nil
	}
	if row.OutboundID == row.ID {
		return errors.New("出站代理不能选择自己")
	}
	target, err := h.repo.GetNodeInbound(row.OutboundID)
	if err != nil {
		return err
	}
	if target == nil {
		return errors.New("所选的出站代理节点不存在")
	}
	if row.ID != 0 && h.chainReaches(target, row.ID) {
		return errors.New("出站代理形成了循环，请检查链路")
	}
	e, err := h.chainEndpoint(target, row.NodeID)
	if err != nil {
		return err
	}
	return kernel.CheckChain(kernelType, e)
}

// chainReaches reports whether following the chain from start arrives at id.
func (h *Handler) chainReaches(start *model.NodeInbound, id int64) bool {
	row := start
	for i := 0; i < maxChainHops && row != nil; i++ {
		if row.ID == id {
			return true
		}
		if row.OutboundID == 0 {
			return false
		}
		next, err := h.repo.GetNodeInbound(row.OutboundID)
		if err != nil {
			return true
		}
		row = next
	}
	return row != nil
}

// chainReferrers returns the inbounds chained through the given one.
func (h *Handler) chainReferrers(targetID int64) []model.NodeInbound {
	rows, err := h.repo.ListInboundsByOutbound(targetID)
	if err != nil {
		log.Printf("chain referrer lookup failed inbound_id=%d err=%v", targetID, err)
		return nil
	}
	return rows
}

// chainInUseMessage explains why an inbound cannot be removed, or "" when
// nothing is chained through it.
func (h *Handler) chainInUseMessage(target *model.NodeInbound) string {
	refs := h.chainReferrers(target.ID)
	if len(refs) == 0 {
		return ""
	}
	names := make([]string, 0, len(refs))
	for _, r := range refs {
		names = append(names, "「"+r.Name+"」")
	}
	return fmt.Sprintf("「%s」正被 %s 用作出站代理，请先解除", target.Name, strings.Join(names, "、"))
}

// applyChainReferrers re-applies the kernels of inbounds chained through the
// target after it changed, returning a warning per failure.
func (h *Handler) applyChainReferrers(targetID int64) []string {
	var warnings []string
	seen := map[int64]bool{}
	for _, ref := range h.chainReferrers(targetID) {
		if isExternalInbound(&ref) || seen[ref.KernelID] {
			continue
		}
		seen[ref.KernelID] = true
		if err := h.applyKernel(ref.KernelID, false); err != nil {
			warnings = append(warnings, fmt.Sprintf("经它出站的「%s」下发失败: %v", ref.Name, err))
		}
	}
	return warnings
}

// applyWhitelistedTarget re-applies a whitelisted target's kernel so its
// allow list follows the nodes chained through it.
func (h *Handler) applyWhitelistedTarget(targetID int64) []string {
	if targetID == 0 {
		return nil
	}
	target, err := h.repo.GetNodeInbound(targetID)
	if err != nil || target == nil || isExternalInbound(target) || target.RelayWhitelist != 1 {
		return nil
	}
	if err := h.applyKernel(target.KernelID, false); err != nil {
		return []string{fmt.Sprintf("出站节点「%s」的白名单下发失败: %v", target.Name, err)}
	}
	return nil
}

// chainDialerNodeIDs returns the nodes that dial the inbound as a chained
// outbound, for its whitelist.
func (h *Handler) chainDialerNodeIDs(inboundID int64) []int64 {
	var ids []int64
	for _, ref := range h.chainReferrers(inboundID) {
		if !isExternalInbound(&ref) {
			ids = append(ids, ref.NodeID)
		}
	}
	return ids
}

// batchDeletesReferrers reports whether a batch delete also removes every
// inbound chained through a target, so the target may go too.
func batchDeletesReferrers(ids []int64, refs []model.NodeInbound) bool {
	deleting := make(map[int64]bool, len(ids))
	for _, id := range ids {
		deleting[id] = true
	}
	for _, r := range refs {
		if !deleting[r.ID] {
			return false
		}
	}
	return true
}
