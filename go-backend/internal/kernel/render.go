package kernel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// File is one configuration file the agent writes before (re)starting a
// kernel. Name is relative to the kernel's run directory.
type File struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// Rendered is everything the agent needs to run one kernel.
type Rendered struct {
	Files []File   `json:"files"`
	Args  []string `json:"args"`
	Hash  string   `json:"hash"`
}

// Render produces the kernel configuration for the given enabled inbounds.
// Inbounds must already be normalized.
func Render(kernelType string, inbounds []Inbound) (Rendered, error) {
	sorted := make([]Inbound, len(inbounds))
	copy(sorted, inbounds)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	var out Rendered
	switch kernelType {
	case TypeXray:
		content, err := renderXray(sorted)
		if err != nil {
			return Rendered{}, err
		}
		out.Files = []File{{Name: "config.json", Content: content}}
		out.Args = []string{"run", "-c", "{RUN_DIR}/config.json"}
	default:
		return Rendered{}, fmt.Errorf("暂不支持内核 %s", kernelType)
	}
	out.Hash = hashRendered(out)
	return out, nil
}

func hashRendered(r Rendered) string {
	h := sha256.New()
	for _, f := range r.Files {
		h.Write([]byte(f.Name))
		h.Write([]byte{0})
		h.Write([]byte(f.Content))
		h.Write([]byte{0})
	}
	for _, a := range r.Args {
		h.Write([]byte(a))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func marshalConfig(v interface{}) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}

// mergeExtra deep-merges a user supplied JSON object into dst. Objects merge
// recursively; any other value replaces the default.
func mergeExtra(dst map[string]interface{}, extra json.RawMessage) error {
	if len(extra) == 0 {
		return nil
	}
	var src map[string]interface{}
	if err := json.Unmarshal(extra, &src); err != nil {
		return fmt.Errorf("高级配置解析失败: %v", err)
	}
	deepMerge(dst, src)
	return nil
}

func deepMerge(dst, src map[string]interface{}) {
	for k, v := range src {
		srcMap, srcIsMap := v.(map[string]interface{})
		dstMap, dstIsMap := dst[k].(map[string]interface{})
		if srcIsMap && dstIsMap {
			deepMerge(dstMap, srcMap)
			continue
		}
		dst[k] = v
	}
}
