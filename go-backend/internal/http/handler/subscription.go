package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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

var subscriptionCheckInterval = time.Minute

// subPipeline is the JSON form of kernel.SubOptions stored per subscription.
type subPipeline struct {
	Lines        string   `json:"lines"`
	Regions      []string `json:"regions"`
	Protocols    []string `json:"protocols"`
	Include      string   `json:"include"`
	Exclude      string   `json:"exclude"`
	Rename       string   `json:"rename"`
	AddFlag      bool     `json:"addFlag"`
	Prefix       string   `json:"prefix"`
	Dedupe       bool     `json:"dedupe"`
	Sort         string   `json:"sort"`
	RegionGroups bool     `json:"regionGroups"`
}

func (p subPipeline) options() kernel.SubOptions {
	return kernel.SubOptions{
		Lines: p.Lines, Regions: p.Regions, Protocols: p.Protocols, Include: p.Include, Exclude: p.Exclude,
		Rename: p.Rename, AddFlag: p.AddFlag, Prefix: p.Prefix, Dedupe: p.Dedupe, Sort: p.Sort,
		RegionGroups: p.RegionGroups,
	}
}

func (p *subPipeline) normalize() error {
	p.Rename = strings.TrimSpace(strings.ReplaceAll(p.Rename, "\r\n", "\n"))
	p.Include = strings.TrimSpace(p.Include)
	p.Exclude = strings.TrimSpace(p.Exclude)
	if len(p.Rename) > 10000 || len(p.Include) > 1000 || len(p.Exclude) > 1000 {
		return errText("规则太长")
	}
	if len([]rune(p.Prefix)) > 50 {
		return errText("前缀最多 50 个字")
	}
	if p.Sort != "name" {
		p.Sort = ""
	}
	if p.Lines != "relay" && p.Lines != "direct" {
		p.Lines = ""
	}
	if p.Regions == nil {
		p.Regions = []string{}
	}
	if p.Protocols == nil {
		p.Protocols = []string{}
	}
	return p.options().Validate()
}

func parsePipeline(raw string) subPipeline {
	var p subPipeline
	_ = json.Unmarshal([]byte(raw), &p)
	if p.Regions == nil {
		p.Regions = []string{}
	}
	if p.Protocols == nil {
		p.Protocols = []string{}
	}
	return p
}

type errText string

func (e errText) Error() string { return string(e) }

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

// subActive reports whether the link works (and, for share subscriptions,
// whether its credentials must be on the kernels).
func subActive(s *model.Subscription, now time.Time) bool {
	return s.Enabled == 1 && (s.ExpireTime == 0 || s.ExpireTime > now.UnixMilli())
}

func isShare(s *model.Subscription) bool { return s.Kind == model.SubscriptionShare }

func kernelUser(s *model.Subscription) kernel.User {
	return kernel.User{ID: s.ID, UUID: s.UUID, Password: s.Password}
}

// subInboundIDs resolves the subscription's sources against the inbounds.
func subInboundIDs(s *model.Subscription, rows []model.NodeInbound) []int64 {
	if s.SourceAll == 1 {
		ids := make([]int64, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.ID)
		}
		return ids
	}
	known := make(map[int64]bool, len(rows))
	for _, r := range rows {
		known[r.ID] = true
	}
	var ids []int64
	for _, id := range parseIDList(s.InboundIDs) {
		if known[id] {
			ids = append(ids, id)
		}
	}
	return ids
}

// activeUsersByInbound maps inbound ids to the share subscriptions whose
// credentials the kernels must currently accept.
func (h *Handler) activeUsersByInbound(now time.Time) map[int64][]kernel.User {
	subs, err := h.repo.ListSubscriptions()
	if err != nil {
		log.Printf("subscription lookup failed: %v", err)
		return nil
	}
	var rows []model.NodeInbound
	out := map[int64][]kernel.User{}
	for i := range subs {
		s := &subs[i]
		if !isShare(s) || !subActive(s, now) {
			continue
		}
		if rows == nil {
			if rows, err = h.repo.ListNodeInbounds(); err != nil {
				return nil
			}
		}
		for _, id := range subInboundIDs(s, rows) {
			out[id] = append(out[id], kernelUser(s))
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

// syncSubscriptions pushes credential changes for share subscriptions
// whose active state differs from what the kernels last received.
func (h *Handler) syncSubscriptions(now time.Time) {
	subs, err := h.repo.ListSubscriptions()
	if err != nil {
		return
	}
	var rows []model.NodeInbound
	for i := range subs {
		s := &subs[i]
		active := boolToInt(isShare(s) && subActive(s, now))
		if active == s.AppliedActive {
			continue
		}
		if rows == nil {
			if rows, err = h.repo.ListNodeInbounds(); err != nil {
				return
			}
		}
		if err := h.applyKernelsForInbounds(subInboundIDs(s, rows)); err != nil {
			log.Printf("subscription %d kernel push failed: %v", s.ID, err)
		}
		_ = h.repo.UpdateSubscriptionApplied(s.ID, active)
	}
}

func (h *Handler) runSubscriptionLoop(ctx context.Context) {
	defer h.jobsWG.Done()
	ticker := time.NewTicker(subscriptionCheckInterval)
	defer ticker.Stop()
	for {
		h.syncSubscriptions(time.Now())
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// subEndpoints are the subscription's endpoints before its pipeline: the
// selected inbounds and their relays, with the share credentials where
// the protocol allows.
func (h *Handler) subEndpoints(s *model.Subscription, rows []model.NodeInbound) []kernel.Endpoint {
	want := map[int64]bool{}
	for _, id := range subInboundIDs(s, rows) {
		want[id] = true
	}
	selected := make([]model.NodeInbound, 0, len(want))
	for _, row := range rows {
		if want[row.ID] {
			selected = append(selected, row)
		}
	}
	endpoints, _ := h.inboundEndpoints(selected)
	if isShare(s) {
		ku := kernelUser(s)
		for i := range endpoints {
			if endpoints[i].Proxy == nil {
				endpoints[i].Inbound = endpoints[i].Inbound.ForUser(ku)
			}
		}
	}
	return endpoints
}

// ---- API ----

type subscriptionView struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Kind       string  `json:"kind"`
	Token      string  `json:"token"`
	Path       string  `json:"path"`
	SourceAll  bool    `json:"sourceAll"`
	InboundIDs []int64 `json:"inboundIds"`
	// SharedIDs are selected inbounds of a share subscription that can't
	// carry per-user credentials (Snell, Shadowsocks, imported nodes).
	SharedIDs  []int64     `json:"sharedIds"`
	Options    subPipeline `json:"options"`
	ExpireTime int64       `json:"expireTime"`
	Enabled    bool        `json:"enabled"`
	Active     bool        `json:"active"`
	Remark     string      `json:"remark"`
	NodeCount  int         `json:"nodeCount"`
}

func (h *Handler) subscriptionView(s *model.Subscription, rows []model.NodeInbound, now time.Time) subscriptionView {
	ids := subInboundIDs(s, rows)
	byID := make(map[int64]model.NodeInbound, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	shared := []int64{}
	if isShare(s) {
		for _, id := range ids {
			row := byID[id]
			if isExternalInbound(&row) {
				shared = append(shared, id)
			} else if in, err := inboundFromModel(row); err == nil && !in.SupportsUsers() {
				shared = append(shared, id)
			}
		}
	}
	selected := ids
	if s.SourceAll == 1 {
		selected = []int64{}
	} else if selected == nil {
		selected = []int64{}
	}
	pipe := parsePipeline(s.Options)
	return subscriptionView{
		ID: s.ID, Name: s.Name, Kind: s.Kind, Token: s.Token, Path: "/api/v1/open_api/sub/" + s.Token,
		SourceAll: s.SourceAll == 1, InboundIDs: selected, SharedIDs: shared, Options: pipe,
		ExpireTime: s.ExpireTime, Enabled: s.Enabled == 1, Active: subActive(s, now), Remark: s.Remark,
		NodeCount: len(kernel.ApplySubOptions(h.subEndpoints(s, rows), pipe.options())),
	}
}

func (h *Handler) subscriptionList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	subs, err := h.repo.ListSubscriptions()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if len(subs) == 0 {
		// Start everyone off with one; deleting it later is the user's call.
		def, err := h.repo.DefaultSubscription(time.Now().UnixMilli())
		if err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
		subs = append(subs, *def)
	}
	rows, err := h.repo.ListNodeInbounds()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	now := time.Now()
	items := make([]subscriptionView, 0, len(subs))
	for i := range subs {
		items = append(items, h.subscriptionView(&subs[i], rows, now))
	}
	response.WriteJSON(w, response.OK(map[string]interface{}{
		"items":   items,
		"regions": append(kernel.RegionNames(), kernel.RegionOther),
	}))
}

type subscriptionRequest struct {
	ID         int64       `json:"id"`
	Name       string      `json:"name"`
	Kind       string      `json:"kind"`
	SourceAll  bool        `json:"sourceAll"`
	InboundIDs []int64     `json:"inboundIds"`
	Options    subPipeline `json:"options"`
	ExpireTime int64       `json:"expireTime"`
	Enabled    *bool       `json:"enabled"`
	Remark     string      `json:"remark"`
}

// validate checks the request and returns the inbound ids it selects.
func (req *subscriptionRequest) validate(rows []model.NodeInbound) ([]int64, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len([]rune(req.Name)) > 100 {
		return nil, errText("请填写名称（最多 100 个字）")
	}
	if req.Kind != model.SubscriptionShare {
		req.Kind = model.SubscriptionOwn
	}
	if req.ExpireTime < 0 {
		req.ExpireTime = 0
	}
	if err := req.Options.normalize(); err != nil {
		return nil, err
	}
	known := make(map[int64]bool, len(rows))
	for _, r := range rows {
		known[r.ID] = true
	}
	var ids []int64
	for _, id := range req.InboundIDs {
		if known[id] {
			ids = append(ids, id)
		}
	}
	if !req.SourceAll && len(ids) == 0 {
		return nil, errText("请至少选择一个入站，或选择全部")
	}
	return ids, nil
}

func (h *Handler) subscriptionSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	var req subscriptionRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	rows, err := h.repo.ListNodeInbounds()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	ids, err := req.validate(rows)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}

	now := time.Now()
	var s *model.Subscription
	var before []int64
	if req.ID > 0 {
		if s, err = h.repo.GetSubscription(req.ID); err != nil || s == nil {
			response.WriteJSON(w, response.ErrDefault("订阅不存在"))
			return
		}
		if isShare(s) {
			before = subInboundIDs(s, rows)
		}
	} else {
		s = &model.Subscription{Token: randomHex(16), Enabled: 1, CreatedTime: now.UnixMilli()}
	}
	s.Name = req.Name
	s.Kind = req.Kind
	if isShare(s) && (s.UUID == "" || s.Password == "") {
		s.UUID, s.Password = newUUID(), randomHex(16)
	}
	s.SourceAll = boolToInt(req.SourceAll)
	s.InboundIDs = formatIDList(ids)
	opts, _ := json.Marshal(req.Options)
	s.Options = string(opts)
	s.ExpireTime = req.ExpireTime
	s.Remark = strings.TrimSpace(req.Remark)
	if req.Enabled != nil {
		s.Enabled = boolToInt(*req.Enabled)
	}
	s.UpdatedTime = now.UnixMilli()
	s.AppliedActive = boolToInt(isShare(s) && subActive(s, now))
	if err := h.repo.SaveSubscription(s); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	push := before
	if isShare(s) {
		push = append(push, subInboundIDs(s, rows)...)
	}
	h.respondAfterSubPush(w, push, h.subscriptionView(s, rows, now))
}

func (h *Handler) respondAfterSubPush(w http.ResponseWriter, inboundIDs []int64, data interface{}) {
	msg := "操作成功"
	if err := h.applyKernelsForInbounds(inboundIDs); err != nil {
		msg = "已保存，但下发到节点失败: " + err.Error()
	}
	response.WriteJSON(w, response.R{Code: 0, Msg: msg, Data: data, TS: time.Now().UnixMilli()})
}

func (h *Handler) subscriptionDelete(w http.ResponseWriter, r *http.Request) {
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	s, err := h.repo.GetSubscription(id)
	if err != nil || s == nil {
		response.WriteJSON(w, response.ErrDefault("订阅不存在"))
		return
	}
	var push []int64
	if isShare(s) {
		if rows, err := h.repo.ListNodeInbounds(); err == nil {
			push = subInboundIDs(s, rows)
		}
	}
	if err := h.repo.DeleteSubscription(id); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	h.respondAfterSubPush(w, push, nil)
}

// subscriptionReset issues a new link; a share subscription also gets new
// credentials, so a leaked link or imported config stops working.
func (h *Handler) subscriptionReset(w http.ResponseWriter, r *http.Request) {
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	s, err := h.repo.GetSubscription(id)
	if err != nil || s == nil {
		response.WriteJSON(w, response.ErrDefault("订阅不存在"))
		return
	}
	s.Token = randomHex(16)
	if isShare(s) {
		s.UUID, s.Password = newUUID(), randomHex(16)
	}
	s.UpdatedTime = time.Now().UnixMilli()
	if err := h.repo.SaveSubscription(s); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	rows, _ := h.repo.ListNodeInbounds()
	var push []int64
	if isShare(s) {
		push = subInboundIDs(s, rows)
	}
	h.respondAfterSubPush(w, push, h.subscriptionView(s, rows, time.Now()))
}

type subPreviewItem struct {
	Name     string `json:"name"`
	Original string `json:"original"`
	Region   string `json:"region"`
	Protocol string `json:"protocol"`
	Line     string `json:"line"` // "direct" or "relay"
}

// subscriptionPreview runs an (unsaved) subscription's pipeline and lists
// the resulting nodes, plus how many the filters dropped.
func (h *Handler) subscriptionPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	var req subscriptionRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	if req.Name == "" {
		req.Name = "preview"
	}
	rows, err := h.repo.ListNodeInbounds()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	ids, err := req.validate(rows)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	draft := &model.Subscription{SourceAll: boolToInt(req.SourceAll), InboundIDs: formatIDList(ids)}
	source := h.subEndpoints(draft, rows)
	result := kernel.ApplySubOptions(source, req.Options.options())
	items := make([]subPreviewItem, 0, len(result))
	for _, e := range result {
		line := "relay"
		if e.Direct {
			line = "direct"
		}
		items = append(items, subPreviewItem{Name: e.Name, Original: e.Original, Region: kernel.RegionOf(e.Name),
			Protocol: kernel.EndpointProtocol(e), Line: line})
	}
	response.WriteJSON(w, response.OK(map[string]interface{}{
		"items":   items,
		"total":   len(source),
		"dropped": len(source) - len(items),
	}))
}

// inboundSubInfo is the pre-3.1 single-subscription API: the default
// subscription's link; reset rotates it.
func (h *Handler) inboundSubInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	var req struct {
		Reset bool `json:"reset"`
	}
	_ = decodeJSON(r.Body, &req)
	s, err := h.repo.DefaultSubscription(time.Now().UnixMilli())
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if req.Reset {
		s.Token = randomHex(16)
		s.UpdatedTime = time.Now().UnixMilli()
		if err := h.repo.SaveSubscription(s); err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
	}
	response.WriteJSON(w, response.OK(map[string]interface{}{
		"token": s.Token,
		"path":  "/api/v1/open_api/sub/" + s.Token,
	}))
}
