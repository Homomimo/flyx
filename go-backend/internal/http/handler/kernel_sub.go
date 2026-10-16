package handler

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"go-backend/internal/http/response"
	"go-backend/internal/kernel"
)

const (
	subConfigRename  = "sub_rename_rules"
	subConfigPrefix  = "sub_name_prefix"
	subConfigSort    = "sub_sort"
	subConfigRegions = "sub_region_groups"
)

type subSettings struct {
	Rename       string `json:"rename"`
	Prefix       string `json:"prefix"`
	Sort         string `json:"sort"`
	RegionGroups bool   `json:"regionGroups"`
}

func (s subSettings) options() kernel.SubOptions {
	return kernel.SubOptions{Rename: s.Rename, Prefix: s.Prefix, Sort: s.Sort, RegionGroups: s.RegionGroups}
}

func (h *Handler) loadSubSettings() subSettings {
	cfg, err := h.repo.ListConfigs()
	if err != nil {
		return subSettings{}
	}
	return subSettings{
		Rename:       cfg[subConfigRename],
		Prefix:       cfg[subConfigPrefix],
		Sort:         cfg[subConfigSort],
		RegionGroups: cfg[subConfigRegions] == "true",
	}
}

// subOptions is applied to every subscription the panel serves.
func (h *Handler) subOptions() kernel.SubOptions {
	return h.loadSubSettings().options()
}

func (s *subSettings) normalize() error {
	s.Rename = strings.TrimSpace(strings.ReplaceAll(s.Rename, "\r\n", "\n"))
	if len(s.Rename) > 10000 {
		return errText("改名规则太长")
	}
	if len([]rune(s.Prefix)) > 50 {
		return errText("前缀最多 50 个字")
	}
	if s.Sort != "" && s.Sort != "name" {
		s.Sort = ""
	}
	_, err := kernel.ParseRenameRules(s.Rename)
	return err
}

type errText string

func (e errText) Error() string { return string(e) }

func (h *Handler) inboundSubSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	response.WriteJSON(w, response.OK(h.loadSubSettings()))
}

func (h *Handler) inboundSubSettingsSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	var req subSettings
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	if err := req.normalize(); err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	now := time.Now().UnixMilli()
	regions := "false"
	if req.RegionGroups {
		regions = "true"
	}
	for k, v := range map[string]string{
		subConfigRename: req.Rename, subConfigPrefix: req.Prefix, subConfigSort: req.Sort, subConfigRegions: regions,
	} {
		if err := h.repo.UpsertConfig(k, v, now); err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
	}
	response.WriteJSON(w, response.OK(req))
}

type subPreviewItem struct {
	Original string `json:"original"`
	Name     string `json:"name"`
	Region   string `json:"region"`
}

// inboundSubPreview shows what the node names become with the given
// (unsaved) settings, using the admin subscription's nodes.
func (h *Handler) inboundSubPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	var req subSettings
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	if err := req.normalize(); err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	rows, err := h.repo.ListNodeInbounds()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	endpoints, _ := h.inboundEndpoints(rows)
	// Rename without sorting so each result lines up with its original,
	// then sort the pairs the same way the subscription does.
	renamed := kernel.ApplySubOptions(endpoints, kernel.SubOptions{Rename: req.Rename, Prefix: req.Prefix})
	out := make([]subPreviewItem, 0, len(renamed))
	for i, e := range renamed {
		item := subPreviewItem{Original: endpoints[i].Name, Name: e.Name}
		if req.RegionGroups {
			item.Region = kernel.RegionOf(e.Name)
		}
		out = append(out, item)
	}
	if req.Sort == "name" {
		sort.SliceStable(out, func(i, j int) bool { return kernel.NaturalLess(out[i].Name, out[j].Name) })
	}
	response.WriteJSON(w, response.OK(out))
}
