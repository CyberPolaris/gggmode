// Package gggmode 根据检测到的画面 tags 反推规则 key（例如 "普通脸——和平精英"、
// "全游戏通用_上号阶段2"），是 Python 版 face_rule_matcher 的 Go 移植。
//
// 匹配策略（两级）：
//  1. 精确命中：某个 key 的 $in tag 全部出现、$nin tag 一个没出现
//     -> 直接返回这些 key（按命中 tag 数降序），不再看模糊结果
//  2. 模糊命中：没有任何精确命中时，按匹配度给每个 key 打分，分数最高的排最前
//     覆盖率 coverage = 命中数 / 规则要求的 tag 数
//     占  比 prec     = 命中数 / 检测到的 tag 数
//     score = 两者的调和平均(F1)；$nin 命中依然是硬性否决
//
// 规则内容（rules.json）由使用方自行传入 LoadRules，本包不内置任何规则。
// 完整说明见 docs/face_rule_matcher.md。
package gggmode

// Version 是当前包版本，发布时与 git tag 保持一致。
const Version = "0.1.0"
