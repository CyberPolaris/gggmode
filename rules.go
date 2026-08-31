package gggmode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// KeySep 是返回 key 的连接符：游戏专属规则返回 "规则名——游戏名"；顶层通用规则原样返回。
// 规则名里请勿使用 "——"。
const KeySep = "——"

// GameIDField 是 JSON 里游戏节点的保留字段名，值为游戏 id（不参与 tag 匹配）。
// match 系列函数的 game 参数传 id 时按它反查游戏名，增删游戏、改 id 只需更新 JSON。
const GameIDField = "game_id"

// SplitKey 把返回的 key 拆成 (规则名, 游戏名)；顶层通用规则没有游戏名，hasGame 为 false。
func SplitKey(key string) (rule, game string, hasGame bool) {
	return strings.Cut(key, KeySep)
}

type opKind int

const (
	opIn opKind = iota
	opMust
	opNin
	opOr
	opAnd
)

var opAlias = map[string]opKind{
	"$in": opIn, "$contains": opIn,
	"$nin": opNin, "$not_contains": opNin,
	"$must": opMust, // 锚点 tag：缺一个整条规则就不成立（精确/模糊都生效）
	"$or":   opOr,
	"$and":  opAnd,
}

// opOf 把 "$in1"、"$contains2" 这类带序号的 key 归一化成标准操作符。
func opOf(key string) (opKind, error) {
	base := strings.TrimRightFunc(key, func(r rune) bool { return r >= '0' && r <= '9' })
	kind, ok := opAlias[base]
	if !ok {
		return 0, fmt.Errorf("未知操作符: %s", key)
	}
	return kind, nil
}

type ruleOp struct {
	kind     opKind
	tags     []string // $in / $must / $nin
	branches []*Rule  // $or：子分支，保持 JSON 顺序（顺序影响并列时取哪一支）
	child    *Rule    // $and
}

// Rule 是一条编译后的匹配规则。同一层级的多个操作符之间是 AND 关系。
type Rule struct {
	ops    []ruleOp
	tagSet map[string]struct{} // 全部 $in/$must tag（$or/$and 递归并集），用于统计 tag 区分度
}

type stageRule struct {
	name string
	rule *Rule
}

type entry struct {
	name   string
	rule   *Rule       // 非 nil 表示顶层通用规则（value 的 key 全部以 $ 开头）
	gameID *int        // 游戏节点的 game_id 字段
	stages []stageRule // 游戏专属规则：阶段名 -> 规则，保持 JSON 顺序
}

// RuleSet 是从 JSON 编译出的规则库。
//
// 规则内容由使用方自行传入（LoadRules），本包不内置任何规则。
// 解析时保持 JSON 键顺序：打分并列时排位靠前的规则先返回，与 Python 版行为一致。
type RuleSet struct {
	entries []*entry
	byName  map[string]*entry
}

// ---- 保序 JSON 解析（encoding/json 的 map 不保序，并列名次会因此抖动） ----

type jsonObj struct {
	keys []string
	vals map[string]any
}

func parseValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	return parseToken(dec, tok)
}

func parseToken(dec *json.Decoder, tok json.Token) (any, error) {
	d, ok := tok.(json.Delim)
	if !ok {
		return tok, nil // string / float64 / bool / nil
	}
	switch d {
	case '{':
		obj := &jsonObj{vals: map[string]any{}}
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key := kt.(string)
			v, err := parseValue(dec)
			if err != nil {
				return nil, err
			}
			if _, dup := obj.vals[key]; !dup {
				obj.keys = append(obj.keys, key)
			}
			obj.vals[key] = v
		}
		if _, err := dec.Token(); err != nil { // 消费 '}'
			return nil, err
		}
		return obj, nil
	case '[':
		arr := []any{}
		for dec.More() {
			v, err := parseValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		if _, err := dec.Token(); err != nil { // 消费 ']'
			return nil, err
		}
		return arr, nil
	}
	return nil, fmt.Errorf("非法 JSON 定界符: %v", d)
}

// isRule 判断：所有 key 都以 $ 开头 => 这是一条规则，而不是 阶段名->规则 的集合。
func isRule(obj *jsonObj) bool {
	if len(obj.keys) == 0 {
		return false
	}
	for _, k := range obj.keys {
		if !strings.HasPrefix(k, "$") {
			return false
		}
	}
	return true
}

func tagList(obj *jsonObj, opKey string) ([]string, error) {
	raw, ok := obj.vals["tags"]
	if !ok {
		return nil, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("操作符 %s 的 tags 必须是数组", opKey)
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("操作符 %s 的 tags 必须是字符串数组", opKey)
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out, nil
}

func compileRule(obj *jsonObj) (*Rule, error) {
	r := &Rule{tagSet: map[string]struct{}{}}
	for _, key := range obj.keys {
		kind, err := opOf(key)
		if err != nil {
			return nil, err
		}
		vobj, ok := obj.vals[key].(*jsonObj)
		if !ok {
			return nil, fmt.Errorf("操作符 %s 的值必须是对象", key)
		}
		switch kind {
		case opIn, opMust, opNin:
			tags, err := tagList(vobj, key)
			if err != nil {
				return nil, err
			}
			r.ops = append(r.ops, ruleOp{kind: kind, tags: tags})
			if kind != opNin {
				for _, t := range tags {
					r.tagSet[t] = struct{}{}
				}
			}
		case opOr:
			op := ruleOp{kind: opOr}
			for _, bk := range vobj.keys {
				branch, err := compileRule(&jsonObj{
					keys: []string{bk},
					vals: map[string]any{bk: vobj.vals[bk]},
				})
				if err != nil {
					return nil, err
				}
				op.branches = append(op.branches, branch)
				for t := range branch.tagSet {
					r.tagSet[t] = struct{}{}
				}
			}
			r.ops = append(r.ops, op)
		case opAnd:
			child, err := compileRule(vobj)
			if err != nil {
				return nil, err
			}
			r.ops = append(r.ops, ruleOp{kind: opAnd, child: child})
			for t := range child.tagSet {
				r.tagSet[t] = struct{}{}
			}
		}
	}
	return r, nil
}

// LoadRules 从 JSON 字节编译规则库。JSON 结构兼容两种写法：
//
//	游戏名 -> 阶段名 -> 规则    如 和平精英 -> 普通脸
//	顶层key -> 规则             全游戏通用规则，如 全游戏通用_上号阶段2
//
// value 的 key 全部以 $ 开头即视为规则；值为空 {} 或只有 game_id 的游戏自动跳过；
// 非对象的顶层值忽略。未知操作符在此处报错（Python 版在匹配时才报）。
func LoadRules(data []byte) (*RuleSet, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	root, err := parseValue(dec)
	if err != nil {
		return nil, fmt.Errorf("解析规则 JSON 失败: %w", err)
	}
	obj, ok := root.(*jsonObj)
	if !ok {
		return nil, fmt.Errorf("规则 JSON 顶层必须是对象")
	}
	rs := &RuleSet{byName: map[string]*entry{}}
	for _, name := range obj.keys {
		node, ok := obj.vals[name].(*jsonObj)
		if !ok {
			continue
		}
		e := &entry{name: name}
		if isRule(node) {
			rule, err := compileRule(node)
			if err != nil {
				return nil, fmt.Errorf("规则 %q: %w", name, err)
			}
			e.rule = rule
		} else {
			if f, ok := node.vals[GameIDField].(float64); ok {
				id := int(f)
				e.gameID = &id
			}
			for _, sub := range node.keys {
				child, ok := node.vals[sub].(*jsonObj)
				if !ok || !isRule(child) {
					continue
				}
				rule, err := compileRule(child)
				if err != nil {
					return nil, fmt.Errorf("规则 %q/%q: %w", name, sub, err)
				}
				e.stages = append(e.stages, stageRule{name: sub, rule: rule})
			}
		}
		rs.entries = append(rs.entries, e)
		rs.byName[name] = e
	}
	return rs, nil
}

// GameIDMap 提取 {游戏名: game_id}，供查看/调试。
func (rs *RuleSet) GameIDMap() map[string]int {
	out := map[string]int{}
	for _, e := range rs.entries {
		if e.rule == nil && e.gameID != nil {
			out[e.name] = *e.gameID
		}
	}
	return out
}

// TagOwners 返回 {tag: [拥有它的规则 key, ...]}（全库统计，key 按规则库顺序）。
// 出现在多条规则里的就是“共用 tag”，只检出它一个时无法区分——用来审查规则库、
// 决定给哪些规则加 $must 锚点。
func (rs *RuleSet) TagOwners() map[string][]string {
	owners := map[string][]string{}
	for _, c := range rs.candidates("", true) {
		for t := range c.rule.tagSet {
			owners[t] = append(owners[t], c.key)
		}
	}
	return owners
}

type candidate struct {
	key  string
	rule *Rule
}

// candidates 展开成 (key, rule)，key 格式全局统一：
// 游戏专属规则 -> "规则名——游戏名"；顶层通用规则 -> 原样。
// game 为空串表示全库搜索；指定 game 时附带顶层通用规则（includeGlobal=false 则只看该游戏）。
func (rs *RuleSet) candidates(game string, includeGlobal bool) []candidate {
	var out []candidate
	add := func(e *entry) {
		if e.rule != nil {
			out = append(out, candidate{e.name, e.rule})
			return
		}
		for _, s := range e.stages {
			out = append(out, candidate{s.name + KeySep + e.name, s.rule})
		}
	}
	if game == "" {
		for _, e := range rs.entries {
			add(e)
		}
		return out
	}
	if e, ok := rs.byName[game]; ok {
		add(e)
	}
	if includeGlobal {
		for _, e := range rs.entries {
			if e.rule != nil && e.name != game {
				out = append(out, candidate{e.name, e.rule})
			}
		}
	}
	return out
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// resolveGame 把 game 参数解析成 rules 里实际的顶层 key；空串表示未知（走通用+全库流程）。
// 支持 nil / 游戏名 / 游戏 id（整数或纯数字字符串）；id 按 game_id 字段反查；
// 名称对不齐时做一次前缀对齐（如 "三角洲行动" -> "三角洲行动手游"）。
// 未登记的 id / 找不到的名称不报错：返回的名字不在库里，调用方按“只匹配通用规则”处理。
func (rs *RuleSet) resolveGame(game any) string {
	var name string
	switch g := game.(type) {
	case nil:
		return ""
	case int:
		return rs.gameByID(g)
	case int8:
		return rs.gameByID(int(g))
	case int16:
		return rs.gameByID(int(g))
	case int32:
		return rs.gameByID(int(g))
	case int64:
		return rs.gameByID(int(g))
	case uint:
		return rs.gameByID(int(g))
	case uint8:
		return rs.gameByID(int(g))
	case uint16:
		return rs.gameByID(int(g))
	case uint32:
		return rs.gameByID(int(g))
	case uint64:
		return rs.gameByID(int(g))
	case string:
		if isDigits(g) {
			id, _ := strconv.Atoi(g)
			return rs.gameByID(id)
		}
		name = g
	default:
		return "" // 不支持的类型按 game 未知处理
	}
	if name == "" {
		return ""
	}
	if _, ok := rs.byName[name]; ok {
		return name
	}
	for _, e := range rs.entries { // 前缀对齐
		if strings.HasPrefix(e.name, name) || strings.HasPrefix(name, e.name) {
			return e.name
		}
	}
	return name
}

func (rs *RuleSet) gameByID(id int) string {
	for _, e := range rs.entries {
		if e.rule == nil && e.gameID != nil && *e.gameID == id {
			return e.name
		}
	}
	return strconv.Itoa(id) // 未登记的 id：不报错，只匹配通用规则
}
