package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"go-backend/internal/http/response"
	"go-backend/internal/kernel"
	"go-backend/internal/store/model"
)

var subUserCheckInterval = time.Minute

func parseIDList(s string) []int64 {
	var out []int64
	seen := map[int64]bool{}
	for _, part := range strings.Split(s, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil && id > 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func formatIDList(ids []int64) string {
	sorted := append([]int64(nil), ids...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	parts := make([]string, len(sorted))
	for i, id := range sorted {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, ",")
}

func subUserActive(u *model.SubUser, now time.Time) bool {
	return u.Enabled == 1 && (u.ExpireTime == 0 || u.ExpireTime > now.UnixMilli())
}

func kernelUser(u *model.SubUser) kernel.User {
	return kernel.User{ID: u.ID, UUID: u.UUID, Password: u.Password}
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// activeUsersByInbound maps inbound ids to the users whose credentials the
// kernels must currently accept.
func (h *Handler) activeUsersByInbound(now time.Time) map[int64][]kernel.User {
	users, err := h.repo.ListSubUsers()
	if err != nil {
		log.Printf("sub user lookup failed: %v", err)
		return nil
	}
	out := map[int64][]kernel.User{}
	for i := range users {
		u := &users[i]
		if !subUserActive(u, now) {
			continue
		}
		for _, id := range parseIDList(u.InboundIDs) {
			out[id] = append(out[id], kernelUser(u))
		}
	}
	return out
}

// applyKernelsForInbounds pushes the kernels serving the given inbounds,
// once each. Imported nodes have no kernel and are skipped.
func (h *Handler) applyKernelsForInbounds(ids []int64) error {
	seen := map[int64]bool{}
	var firstErr error
	for _, id := range ids {
		row, err := h.repo.GetNodeInbound(id)
		if err != nil || row == nil || isExternalInbound(row) || seen[row.KernelID] {
			continue
		}
		seen[row.KernelID] = true
		if err := h.applyKernel(row.KernelID, false); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// syncSubUsers pushes credential changes for users whose active state
// differs from what the kernels last received (expiry, mostly).
func (h *Handler) syncSubUsers(now time.Time) {
	users, err := h.repo.ListSubUsers()
	if err != nil {
		return
	}
	for i := range users {
		u := &users[i]
		active := boolToInt(subUserActive(u, now))
		if active == u.AppliedActive {
			continue
		}
		if err := h.applyKernelsForInbounds(parseIDList(u.InboundIDs)); err != nil {
			log.Printf("sub user %d kernel push failed: %v", u.ID, err)
		}
		_ = h.repo.UpdateSubUserApplied(u.ID, active)
	}
}

func (h *Handler) runSubUserLoop(ctx context.Context) {
	defer h.jobsWG.Done()
	ticker := time.NewTicker(subUserCheckInterval)
	defer ticker.Stop()
	for {
		h.syncSubUsers(time.Now())
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ---- API ----

type subUserView struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Token      string  `json:"token"`
	Path       string  `json:"path"`
	InboundIDs []int64 `json:"inboundIds"`
	// SharedIDs are selected inbounds that can't carry per-user
	// credentials (Snell, Shadowsocks, imported nodes): the user gets the
	// shared credentials and disabling the user can't cut them off.
	SharedIDs  []int64 `json:"sharedIds"`
	ExpireTime int64   `json:"expireTime"`
	Enabled    bool    `json:"enabled"`
	Active     bool    `json:"active"`
	Remark     string  `json:"remark"`
}

func (h *Handler) subUserView(u *model.SubUser, rows map[int64]model.NodeInbound, now time.Time) subUserView {
	ids := parseIDList(u.InboundIDs)
	var kept, shared []int64
	for _, id := range ids {
		row, ok := rows[id]
		if !ok {
			continue
		}
		kept = append(kept, id)
		if isExternalInbound(&row) {
			shared = append(shared, id)
			continue
		}
		if in, err := inboundFromModel(row); err == nil && !in.SupportsUsers() {
			shared = append(shared, id)
		}
	}
	if kept == nil {
		kept = []int64{}
	}
	if shared == nil {
		shared = []int64{}
	}
	return subUserView{
		ID: u.ID, Name: u.Name, Token: u.Token, Path: "/api/v1/open_api/sub/" + u.Token,
		InboundIDs: kept, SharedIDs: shared, ExpireTime: u.ExpireTime,
		Enabled: u.Enabled == 1, Active: subUserActive(u, now), Remark: u.Remark,
	}
}

func (h *Handler) inboundRowsByID() (map[int64]model.NodeInbound, error) {
	rows, err := h.repo.ListNodeInbounds()
	if err != nil {
		return nil, err
	}
	out := make(map[int64]model.NodeInbound, len(rows))
	for _, row := range rows {
		out[row.ID] = row
	}
	return out, nil
}

func (h *Handler) subUserList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	users, err := h.repo.ListSubUsers()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	rows, err := h.inboundRowsByID()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	now := time.Now()
	out := make([]subUserView, 0, len(users))
	for i := range users {
		out = append(out, h.subUserView(&users[i], rows, now))
	}
	response.WriteJSON(w, response.OK(out))
}

type subUserRequest struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	InboundIDs []int64 `json:"inboundIds"`
	ExpireTime int64   `json:"expireTime"`
	Enabled    *bool   `json:"enabled"`
	Remark     string  `json:"remark"`
}

func (h *Handler) subUserSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	var req subUserRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len([]rune(req.Name)) > 100 {
		response.WriteJSON(w, response.ErrDefault("请填写名称（最多 100 个字）"))
		return
	}
	if req.ExpireTime < 0 {
		req.ExpireTime = 0
	}
	rows, err := h.inboundRowsByID()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	var ids []int64
	for _, id := range req.InboundIDs {
		if _, ok := rows[id]; ok {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		response.WriteJSON(w, response.ErrDefault("请至少选择一个入站"))
		return
	}

	now := time.Now()
	var u *model.SubUser
	var before []int64
	if req.ID > 0 {
		u, err = h.repo.GetSubUser(req.ID)
		if err != nil || u == nil {
			response.WriteJSON(w, response.ErrDefault("用户不存在"))
			return
		}
		before = parseIDList(u.InboundIDs)
	} else {
		u = &model.SubUser{Token: randomHex(16), UUID: newUUID(), Password: randomHex(16),
			Enabled: 1, CreatedTime: now.UnixMilli()}
	}
	u.Name = req.Name
	u.InboundIDs = formatIDList(ids)
	u.ExpireTime = req.ExpireTime
	u.Remark = strings.TrimSpace(req.Remark)
	if req.Enabled != nil {
		u.Enabled = boolToInt(*req.Enabled)
	}
	u.UpdatedTime = now.UnixMilli()
	u.AppliedActive = boolToInt(subUserActive(u, now))
	if err := h.repo.SaveSubUser(u); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	h.respondAfterUserPush(w, append(before, ids...), h.subUserView(u, rows, now))
}

func (h *Handler) respondAfterUserPush(w http.ResponseWriter, inboundIDs []int64, data interface{}) {
	msg := "操作成功"
	if err := h.applyKernelsForInbounds(inboundIDs); err != nil {
		msg = "已保存，但下发到节点失败: " + err.Error()
	}
	response.WriteJSON(w, response.R{Code: 0, Msg: msg, Data: data, TS: time.Now().UnixMilli()})
}

func (h *Handler) subUserDelete(w http.ResponseWriter, r *http.Request) {
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	u, err := h.repo.GetSubUser(id)
	if err != nil || u == nil {
		response.WriteJSON(w, response.ErrDefault("用户不存在"))
		return
	}
	if err := h.repo.DeleteSubUser(id); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	h.respondAfterUserPush(w, parseIDList(u.InboundIDs), nil)
}

// subUserReset issues a new link and new credentials, so a leaked link or
// config stops working.
func (h *Handler) subUserReset(w http.ResponseWriter, r *http.Request) {
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	u, err := h.repo.GetSubUser(id)
	if err != nil || u == nil {
		response.WriteJSON(w, response.ErrDefault("用户不存在"))
		return
	}
	u.Token, u.UUID, u.Password = randomHex(16), newUUID(), randomHex(16)
	u.UpdatedTime = time.Now().UnixMilli()
	if err := h.repo.SaveSubUser(u); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	rows, _ := h.inboundRowsByID()
	h.respondAfterUserPush(w, parseIDList(u.InboundIDs), h.subUserView(u, rows, time.Now()))
}

// userEndpoints are the user's shared inbounds as the user connects to
// them: own credentials where supported, shared ones otherwise.
func (h *Handler) userEndpoints(u *model.SubUser, rows []model.NodeInbound) []kernel.Endpoint {
	want := map[int64]bool{}
	for _, id := range parseIDList(u.InboundIDs) {
		want[id] = true
	}
	selected := make([]model.NodeInbound, 0, len(want))
	for _, row := range rows {
		if want[row.ID] {
			selected = append(selected, row)
		}
	}
	endpoints, _ := h.inboundEndpoints(selected)
	ku := kernelUser(u)
	for i := range endpoints {
		if endpoints[i].Proxy == nil {
			endpoints[i].Inbound = endpoints[i].Inbound.ForUser(ku)
		}
	}
	return endpoints
}
