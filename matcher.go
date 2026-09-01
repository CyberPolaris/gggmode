package gggmode

import (
	"math"
	"sort"
)

// Detail 是单条命中明细：hit=对上的 tag 数，total=规则要求的 tag 总数，
// coverage=hit/total，prec=hit/检出tag数，score=两者的调和平均(F1)，
// exact=hit==total（精确命中）。数值与 Python 版一致，保留三位小数。
type Detail struct {
	Key      string  `json:"key"`
	Hit      int     `json:"hit"`
	Total    int     `json:"total"`
	Coverage float64 `json:"coverage"`
	Prec     float64 `json:"prec"`
	Score    float64 `json:"score"`
	Exact    bool    `json:"exact"`
}

// Options 是匹配参数，零值不可直接使用，请通过 With* 选项在默认值上调整。
type Options struct {
	// Fuzzy 是否允许模糊命中（默认 true）。false 时只认精确命中。
	Fuzzy bool
	// MinHits 模糊命中至少要对上的 tag 个数（默认 2，防止一两个 tag 巧合命中）。
	// 严格门槛，不随检出数收缩：检出 tag 数不足 MinHits 时模糊匹配必然不认，
	// 确实要认单 tag 的场景显式传 WithMinHits(1)。
	MinHits int
	// MinPrec 模糊入围门槛 = 命中数 / 检出tag数（默认 0.5）。
	// 不看覆盖率，规则含 tag 再多也不吃亏；score(F1) 只用于排序和展示。
	MinPrec float64
	// IncludeGlobal 指定 game 时是否连同顶层通用规则一起匹配（默认 true）。
	IncludeGlobal bool
	// ExactFirst true（默认）= 精确命中提第一梯队；false = 纯模糊模式：
	// 所有候选统一过 MinHits/MinPrec 门槛、统一按 score 排序（主要用于对比实验）。
	ExactFirst bool
	// UniqueSingle 已废弃，无任何效果，保留仅为兼容：严格 MinHits 门槛下
	// 不可能出现 hit < MinHits 的入围项，防护无用武之地。
	//
	// Deprecated: 严格 MinHits 已覆盖其功能。
	UniqueSingle bool
}

func defaultOptions() Options {
	return Options{
		Fuzzy:         true,
		MinHits:       2,
		MinPrec:       0.5,
		IncludeGlobal: true,
		ExactFirst:    true,
		UniqueSingle:  true,
	}
}

// Option 按函数式选项模式调整匹配参数。
type Option func(*Options)

// WithFuzzy 设置是否允许模糊命中（默认 true）。
func WithFuzzy(v bool) Option { return func(o *Options) { o.Fuzzy = v } }

// WithMinHits 设置模糊命中最少 tag 数（默认 2）。
func WithMinHits(v int) Option { return func(o *Options) { o.MinHits = v } }

// WithMinPrec 设置模糊入围的最低占比（默认 0.5）。
func WithMinPrec(v float64) Option { return func(o *Options) { o.MinPrec = v } }

// WithIncludeGlobal 设置指定 game 时是否附带顶层通用规则（默认 true）。
func WithIncludeGlobal(v bool) Option { return func(o *Options) { o.IncludeGlobal = v } }

// WithExactFirst 设置精确命中是否提第一梯队（默认 true）。
func WithExactFirst(v bool) Option { return func(o *Options) { o.ExactFirst = v } }

// WithUniqueSingle 已废弃，无任何效果，保留仅为兼容。
//
// Deprecated: 严格 MinHits 门槛已覆盖其功能，见 Options.UniqueSingle。
func WithUniqueSingle(v bool) Option { return func(o *Options) { o.UniqueSingle = v } }

func round3(f float64) float64 { return math.Round(f*1000) / 1000 }

func covOf(hit, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(hit) / float64(total)
}

// score 递归统计 (hit, total)。检测结果里出现了 $nin 禁止的 tag、或缺了 $must
// 锚点 tag 时 ok=false（整条规则否决）。$or 取覆盖率最高（并列取 hit 大）的分支。
func (r *Rule) score(tags map[string]struct{}) (hit, total int, ok bool) {
	for _, op := range r.ops {
		switch op.kind {
		case opIn:
			for _, t := range op.tags {
				if _, has := tags[t]; has {
					hit++
				}
			}
			total += len(op.tags)
		case opMust: // 锚点：必须全部出现，否则整条规则不成立
			for _, t := range op.tags {
				if _, has := tags[t]; !has {
					return 0, 0, false
				}
			}
			hit += len(op.tags)
			total += len(op.tags)
		case opNin:
			for _, t := range op.tags {
				if _, has := tags[t]; has {
					return 0, 0, false
				}
			}
		case opOr:
			bh, bt, found := 0, 0, false
			for _, br := range op.branches {
				h, t, k := br.score(tags)
				if !k {
					continue
				}
				if !found || covOf(h, t) > covOf(bh, bt) ||
					(covOf(h, t) == covOf(bh, bt) && h > bh) {
					bh, bt, found = h, t, true
				}
			}
			if !found {
				return 0, 0, false
			}
			hit += bh
			total += bt
		case opAnd:
			h, t, k := op.child.score(tags)
			if !k {
				return 0, 0, false
			}
			hit += h
			total += t
		}
	}
	return hit, total, true
}

// MatchDetail 返回全部命中及打分明细，没命中返回空切片；用于调阈值/排查误判/打日志。
// 有精确命中时只返回精确命中（按 hit 降序）；否则返回达到门槛的模糊命中（按 score 降序）。
//
// game 参数：
//
//	已登记的 id / 名称   候选 = 该游戏的专属规则 + 顶层通用规则（严格作用域，不跨游戏兜底）
//	game 未知            nil / 未登记的 id / 未登记的名称，行为完全一致：
//	                     先只看顶层通用规则，“全中”才就地返回（游戏未知时不冒认游戏名）；
//	                     否则退回全库反查
//
// 传了 id 却命中别的游戏（key 带 "——游戏名" 后缀），说明 id 和画面对不上。
func (rs *RuleSet) MatchDetail(game any, detectedTags []string, opts ...Option) []Detail {
	o := defaultOptions()
	for _, fn := range opts {
		fn(&o)
	}
	tags := make(map[string]struct{}, len(detectedTags))
	for _, t := range detectedTags {
		tags[t] = struct{}{}
	}
	effMinHits := o.MinHits // 严格门槛：不随检出数收缩，证据不足一律不认
	if effMinHits < 1 {
		effMinHits = 1
	}

	// run 对一批候选统一打分、分层、排序
	run := func(cands []candidate) []Detail {
		var exact, partial []Detail
		for _, c := range cands {
			hit, total, ok := c.rule.score(tags)
			if !ok || total == 0 {
				continue
			}
			cov := float64(hit) / float64(total)
			prec := 0.0
			if len(tags) > 0 {
				prec = float64(hit) / float64(len(tags))
			}
			score := 0.0
			if cov > 0 && prec > 0 {
				score = 2 * cov * prec / (cov + prec)
			}
			d := Detail{c.key, hit, total, round3(cov), round3(prec), round3(score), hit == total}
			switch {
			case o.ExactFirst && d.Exact:
				exact = append(exact, d)
			case o.Fuzzy && hit >= effMinHits && prec >= o.MinPrec:
				partial = append(partial, d)
			}
		}
		if len(exact) > 0 {
			sort.SliceStable(exact, func(i, j int) bool {
				if exact[i].Hit != exact[j].Hit {
					return exact[i].Hit > exact[j].Hit
				}
				return exact[i].Score > exact[j].Score
			})
			return exact
		}
		sort.SliceStable(partial, func(i, j int) bool {
			if partial[i].Score != partial[j].Score {
				return partial[i].Score > partial[j].Score
			}
			return partial[i].Hit > partial[j].Hit
		})
		return partial
	}

	resolved := rs.resolveGame(game)
	if resolved != "" {
		if _, registered := rs.byName[resolved]; registered {
			// 已登记的游戏：严格作用域（该游戏专属规则 + 通用规则），不跨游戏兜底
			return run(rs.candidates(resolved, o.IncludeGlobal))
		}
	}

	// game 未知：第一段只看顶层通用规则，“全中”才有资格就地返回
	var generics []candidate
	for _, e := range rs.entries {
		if e.rule != nil {
			generics = append(generics, candidate{e.name, e.rule})
		}
	}
	generic := run(generics)
	for _, d := range generic {
		if d.Exact {
			return generic
		}
	}
	// 第二段：全库反查。候选是通用规则的超集，第一段的模糊结果会在这里重新排位；
	// 命中某个游戏时，key 的 "——游戏名" 后缀会告诉你画面实际属于谁
	return run(rs.candidates("", o.IncludeGlobal))
}

// MatchKeys 只要 key 列表（顺序与 MatchDetail 一致），没命中返回空切片。
func (rs *RuleSet) MatchKeys(game any, detectedTags []string, opts ...Option) []string {
	details := rs.MatchDetail(game, detectedTags, opts...)
	keys := make([]string, len(details))
	for i, d := range details {
		keys[i] = d.Key
	}
	return keys
}

// BestMatch 只取排第一的 key；没命中时 ok 为 false。生产流水线判画面用。
func (rs *RuleSet) BestMatch(game any, detectedTags []string, opts ...Option) (key string, ok bool) {
	details := rs.MatchDetail(game, detectedTags, opts...)
	if len(details) == 0 {
		return "", false
	}
	return details[0].Key, true
}
