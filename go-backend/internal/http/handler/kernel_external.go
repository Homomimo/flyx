package handler

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go-backend/internal/http/response"
	"go-backend/internal/kernel"
	"go-backend/internal/store/model"
)

// External inbounds are proxy nodes the panel does not deploy (built by a
// script, or on a machine without the agent). They are stored as inbounds
// without a kernel or node; Listen holds the node's public address so relays
// and subscriptions treat them like any other inbound.

const maxImportLinks = 100

func isExternalInbound(row *model.NodeInbound) bool {
	return row != nil && row.KernelID == 0
}

type externalImportResult struct {
	Created []int64  `json:"created"`
	Errors  []string `json:"errors"`
}

// inboundImport creates external inbounds from share links, one per line.
func (h *Handler) inboundImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	var req struct {
		Links     string `json:"links"`
		RelayOnly bool   `json:"relayOnly"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	var lines []string
	for _, line := range strings.Split(req.Links, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		response.WriteJSON(w, response.ErrDefault("请粘贴分享链接，每行一个"))
		return
	}
	if len(lines) > maxImportLinks {
		response.WriteJSON(w, response.ErrDefault(fmt.Sprintf("一次最多导入 %d 条", maxImportLinks)))
		return
	}

	existing, err := h.repo.ListNodeInbounds()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	seen := make(map[string]bool)
	for _, row := range existing {
		if isExternalInbound(&row) {
			seen[externalKey(row.Listen, row.Port)] = true
		}
	}

	res := externalImportResult{Created: []int64{}, Errors: []string{}}
	now := time.Now().UnixMilli()
	for i, line := range lines {
		node, err := kernel.ParseShareLink(line)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("第 %d 行：%v", i+1, err))
			continue
		}
		key := externalKey(node.Host, node.Port)
		if seen[key] {
			res.Errors = append(res.Errors, fmt.Sprintf("第 %d 行：%s 已经导入过", i+1, key))
			continue
		}
		seen[key] = true
		row := model.NodeInbound{
			Name:        truncateRunes(node.Name, 100),
			ShareHost:   node.Host,
			RelayOnly:   boolToInt(req.RelayOnly),
			Enabled:     1,
			CreatedTime: now,
			UpdatedTime: now,
		}
		if err := applyInboundToModel(node.Inbound, &row); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("第 %d 行：%v", i+1, err))
			continue
		}
		if err := h.repo.CreateNodeInbound(&row); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("第 %d 行：%v", i+1, err))
			continue
		}
		res.Created = append(res.Created, row.ID)
	}
	if len(res.Created) == 0 {
		response.WriteJSON(w, response.R{Code: -1, Msg: "没有导入任何节点：" + strings.Join(res.Errors, "；"), Data: res, TS: now})
		return
	}
	response.WriteJSON(w, response.OK(res))
}

// inboundExternalUpdate edits an imported node's address and credentials.
func (h *Handler) inboundExternalUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	var req struct {
		ID        int64  `json:"id"`
		Name      string `json:"name"`
		Host      string `json:"host"`
		Port      int    `json:"port"`
		Method    string `json:"method"`
		Password  string `json:"password"`
		Remark    string `json:"remark"`
		RelayOnly bool   `json:"relayOnly"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	row, err := h.repo.GetNodeInbound(req.ID)
	if err != nil || row == nil || !isExternalInbound(row) {
		response.WriteJSON(w, response.ErrDefault("导入的节点不存在"))
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len([]rune(name)) > 100 {
		response.WriteJSON(w, response.ErrDefault("名称不能为空，且不超过 100 字"))
		return
	}
	host := strings.Trim(strings.TrimSpace(req.Host), "[]")
	if err := kernel.ValidateImportHost(host); err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	if req.Port < 1 || req.Port > 65535 {
		response.WriteJSON(w, response.ErrDefault("端口不正确"))
		return
	}
	method := strings.ToLower(strings.TrimSpace(req.Method))
	if !containsMethod(method) {
		response.WriteJSON(w, response.ErrDefault("不支持的加密方式 "+method))
		return
	}
	if req.Password == "" {
		response.WriteJSON(w, response.ErrDefault("密码不能为空"))
		return
	}

	in, err := inboundFromModel(*row)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	in.Listen, in.Port = host, req.Port
	in.Settings.Method, in.Settings.Password = method, req.Password
	if err := applyInboundToModel(in, row); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	row.Name = name
	row.ShareHost = host
	row.Remark = strings.TrimSpace(req.Remark)
	row.RelayOnly = boolToInt(req.RelayOnly)
	row.UpdatedTime = time.Now().UnixMilli()
	if err := h.repo.SaveNodeInbound(row); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	// Relay forwards dial the stored address; point them at the new one.
	if problems := h.retargetRelays(row); len(problems) > 0 {
		response.WriteJSON(w, response.R{Code: 0, Msg: "已保存，但部分中转更新失败: " + strings.Join(problems, "；"), TS: row.UpdatedTime})
		return
	}
	response.WriteJSON(w, response.OKEmpty())
}

func externalKey(host string, port int) string {
	return net.JoinHostPort(strings.ToLower(host), strconv.Itoa(port))
}

func containsMethod(method string) bool {
	for _, m := range kernel.SSMethods() {
		if m == method {
			return true
		}
	}
	return false
}

// retargetRelays updates the remote address of every managed relay forward
// of an inbound after its address changed.
func (h *Handler) retargetRelays(row *model.NodeInbound) []string {
	relays, err := h.repo.ListInboundRelays(row.ID)
	if err != nil {
		return []string{err.Error()}
	}
	var problems []string
	for _, rel := range relays {
		if !rel.ForwardExists || rel.Managed != 1 {
			continue
		}
		fwd, err := h.getForwardRecord(rel.ForwardID)
		if err != nil || fwd == nil {
			continue
		}
		tunnel, err := h.getTunnelRecord(fwd.TunnelID)
		if err != nil || tunnel == nil {
			continue
		}
		target, err := h.relayTarget(row, tunnel)
		if err != nil {
			problems = append(problems, rel.Name+": "+err.Error())
			continue
		}
		if target == fwd.RemoteAddr {
			continue
		}
		if err := h.repo.UpdateForward(fwd.ID, fwd.Name, fwd.TunnelID, target, fwd.Strategy, time.Now().UnixMilli(),
			fwd.SpeedID, fwd.MaxConn, fwd.IPMaxConn, fwd.IPSpeedID, 0, fwd.ProxyProtocolReceive, fwd.ProxyProtocolSend); err != nil {
			problems = append(problems, rel.Name+": "+err.Error())
			continue
		}
		updated, err := h.getForwardRecord(fwd.ID)
		if err != nil || updated == nil {
			continue
		}
		if err := h.syncForwardServices(updated, "UpdateService", true); err != nil {
			problems = append(problems, rel.Name+": "+err.Error())
		}
	}
	return problems
}

func truncateRunes(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}
