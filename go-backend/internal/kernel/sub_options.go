package kernel

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// SubOptions customizes the node list of a subscription.
type SubOptions struct {
	// Rename rules, one per line: "find => replace". A find wrapped in
	// slashes (/.../) is a regular expression; an empty replace deletes.
	Rename string
	Prefix string
	// Sort is "" (panel order) or "name".
	Sort string
	// RegionGroups adds one auto-select group per detected region.
	RegionGroups bool
}

type renameRule struct {
	re   *regexp.Regexp
	find string
	repl string
}

// ParseRenameRules parses the rule text and reports the first bad line.
func ParseRenameRules(text string) ([]renameRule, error) {
	var rules []renameRule
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=>", 2)
		find := strings.TrimSpace(parts[0])
		repl := ""
		if len(parts) == 2 {
			repl = strings.TrimSpace(parts[1])
		}
		if find == "" {
			return nil, fmt.Errorf("第 %d 行：缺少要替换的内容", i+1)
		}
		rule := renameRule{find: find, repl: repl}
		if len(find) > 2 && strings.HasPrefix(find, "/") && strings.HasSuffix(find, "/") {
			re, err := regexp.Compile(find[1 : len(find)-1])
			if err != nil {
				return nil, fmt.Errorf("第 %d 行：正则表达式错误", i+1)
			}
			rule.re = re
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// ApplySubOptions renames and sorts endpoints. Invalid rename rules are
// skipped (they are validated when saved).
func ApplySubOptions(endpoints []Endpoint, opts SubOptions) []Endpoint {
	rules, _ := ParseRenameRules(opts.Rename)
	out := make([]Endpoint, len(endpoints))
	for i, e := range endpoints {
		name := e.Name
		for _, r := range rules {
			if r.re != nil {
				name = r.re.ReplaceAllString(name, r.repl)
			} else {
				name = strings.ReplaceAll(name, r.find, r.repl)
			}
		}
		name = strings.TrimSpace(name)
		if name == "" {
			name = strings.TrimSpace(e.Name)
		}
		e.Name = opts.Prefix + name
		out[i] = e
	}
	if opts.Sort == "name" {
		sort.SliceStable(out, func(i, j int) bool { return NaturalLess(out[i].Name, out[j].Name) })
	}
	return out
}

// NaturalLess compares names with digit runs compared numerically, so
// "香港 2" sorts before "香港 10".
func NaturalLess(a, b string) bool {
	ra, rb := []rune(a), []rune(b)
	i, j := 0, 0
	for i < len(ra) && j < len(rb) {
		if isDigit(ra[i]) && isDigit(rb[j]) {
			si := i
			for i < len(ra) && isDigit(ra[i]) {
				i++
			}
			sj := j
			for j < len(rb) && isDigit(rb[j]) {
				j++
			}
			na := strings.TrimLeft(string(ra[si:i]), "0")
			nb := strings.TrimLeft(string(rb[sj:j]), "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		if ra[i] != rb[j] {
			return ra[i] < rb[j]
		}
		i++
		j++
	}
	return len(ra)-i < len(rb)-j
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

type region struct {
	name     string
	keywords []string
}

// regions are matched in order; keywords are matched case-insensitively,
// short ASCII codes only as whole words.
var regions = []region{
	{"🇭🇰 香港", []string{"🇭🇰", "香港", "港", "hong kong", "hongkong", "hk"}},
	{"🇹🇼 台湾", []string{"🇹🇼", "台湾", "台灣", "taiwan", "tw"}},
	{"🇯🇵 日本", []string{"🇯🇵", "日本", "东京", "大阪", "japan", "tokyo", "osaka", "jp"}},
	{"🇸🇬 新加坡", []string{"🇸🇬", "新加坡", "狮城", "singapore", "sg"}},
	{"🇰🇷 韩国", []string{"🇰🇷", "韩国", "韓國", "首尔", "korea", "seoul", "kr"}},
	{"🇺🇸 美国", []string{"🇺🇸", "美国", "美國", "洛杉矶", "圣何塞", "united states", "los angeles", "us", "usa"}},
	{"🇬🇧 英国", []string{"🇬🇧", "英国", "伦敦", "united kingdom", "london", "uk", "gb"}},
	{"🇩🇪 德国", []string{"🇩🇪", "德国", "法兰克福", "germany", "frankfurt", "de"}},
	{"🇫🇷 法国", []string{"🇫🇷", "法国", "巴黎", "france", "paris", "fr"}},
	{"🇳🇱 荷兰", []string{"🇳🇱", "荷兰", "阿姆斯特丹", "netherlands", "amsterdam", "nl"}},
	{"🇨🇦 加拿大", []string{"🇨🇦", "加拿大", "canada", "ca"}},
	{"🇦🇺 澳大利亚", []string{"🇦🇺", "澳大利亚", "澳洲", "悉尼", "australia", "sydney", "au"}},
	{"🇷🇺 俄罗斯", []string{"🇷🇺", "俄罗斯", "莫斯科", "russia", "moscow", "ru"}},
	{"🇮🇳 印度", []string{"🇮🇳", "印度", "india", "in"}},
	{"🇹🇷 土耳其", []string{"🇹🇷", "土耳其", "turkey", "tr"}},
	{"🇻🇳 越南", []string{"🇻🇳", "越南", "vietnam", "vn"}},
	{"🇹🇭 泰国", []string{"🇹🇭", "泰国", "thailand", "th"}},
	{"🇵🇭 菲律宾", []string{"🇵🇭", "菲律宾", "philippines", "ph"}},
	{"🇲🇾 马来西亚", []string{"🇲🇾", "马来西亚", "malaysia", "my"}},
	{"🇦🇷 阿根廷", []string{"🇦🇷", "阿根廷", "argentina", "ar"}},
	{"🇧🇷 巴西", []string{"🇧🇷", "巴西", "brazil", "br"}},
	{"🇦🇪 阿联酋", []string{"🇦🇪", "阿联酋", "迪拜", "dubai", "uae"}},
}

var wordSplit = regexp.MustCompile(`[^a-z0-9]+`)

// RegionOf returns the region group name for an endpoint name, or "".
func RegionOf(name string) string {
	lower := strings.ToLower(name)
	words := map[string]bool{}
	for _, w := range wordSplit.Split(lower, -1) {
		if w != "" {
			words[w] = true
		}
	}
	for _, r := range regions {
		for _, k := range r.keywords {
			ascii := true
			for _, c := range k {
				if c > 127 {
					ascii = false
					break
				}
			}
			if ascii && !strings.Contains(k, " ") && len(k) <= 3 {
				if words[k] {
					return r.name
				}
				continue
			}
			if strings.Contains(lower, k) {
				return r.name
			}
		}
	}
	return ""
}

type regionGroup struct {
	name    string
	members []string
}

// regionGroups groups names by region, in the order of the region table.
// Regions with no nodes are left out.
func regionGroups(names []string) []regionGroup {
	byRegion := map[string][]string{}
	for _, n := range names {
		if r := RegionOf(n); r != "" {
			byRegion[r] = append(byRegion[r], n)
		}
	}
	var out []regionGroup
	for _, r := range regions {
		if m := byRegion[r.name]; len(m) > 0 {
			out = append(out, regionGroup{name: r.name, members: m})
		}
	}
	return out
}
