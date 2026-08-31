package gggmode

import (
	"os"
	"reflect"
	"testing"
)

// 测试用例与期望值全部取自 Python 原版 face_rule_matcher.py 对同一份
// testdata/rules.json 的实际运行输出（例 1~例 8），保证移植行为一致。

func loadTestRules(t *testing.T) *RuleSet {
	t.Helper()
	data, err := os.ReadFile("testdata/rules.json")
	if err != nil {
		t.Fatalf("读取 testdata/rules.json 失败: %v", err)
	}
	rs, err := LoadRules(data)
	if err != nil {
		t.Fatalf("LoadRules 失败: %v", err)
	}
	return rs
}

func TestGameIDMap(t *testing.T) {
	rs := loadTestRules(t)
	// 这份 rules.json 没有 game_id 字段
	if m := rs.GameIDMap(); len(m) != 0 {
		t.Fatalf("GameIDMap = %v, 期望为空", m)
	}
}

// 例 1：普通脸的画面（精确命中）；id 683 未登记，走全库反查仍应精确命中
func TestExactMatch(t *testing.T) {
	rs := loadTestRules(t)
	tags := []string{
		"腾讯健康系统_人脸验证窗口", "腾讯健康系统_人脸验证标记",
		"腾讯健康系统_开始验证", "腾讯健康系统_暂不验证",
	}
	want := []string{"普通脸——和平精英"}
	if got := rs.MatchKeys(683, tags); !reflect.DeepEqual(got, want) {
		t.Fatalf("MatchKeys = %v, 期望 %v", got, want)
	}
	// 指定游戏名（严格作用域）结果一致
	if got := rs.MatchKeys("和平精英", tags); !reflect.DeepEqual(got, want) {
		t.Fatalf("MatchKeys(和平精英) = %v, 期望 %v", got, want)
	}
}

// 例 2：多出倒计时 -> 普通脸被 $nin 否决，通用规则 腾讯健康系统 精确命中
func TestNinVeto(t *testing.T) {
	rs := loadTestRules(t)
	tags := []string{
		"腾讯健康系统_人脸验证窗口", "腾讯健康系统_人脸验证标记",
		"腾讯健康系统_开始验证", "腾讯健康系统_暂不验证", "腾讯健康系统_人脸倒计时",
	}
	want := []string{"腾讯健康系统"}
	if got := rs.MatchKeys(683, tags); !reflect.DeepEqual(got, want) {
		t.Fatalf("MatchKeys = %v, 期望 %v", got, want)
	}
}

// 例 3：设备脸的第二种画面（$or 分支）
func TestOrBranch(t *testing.T) {
	rs := loadTestRules(t)
	tags := []string{
		"和平精英_腾讯游戏身份验证", "和平精英_腾讯游戏身份验证完成开始按钮",
		"和平精英_腾讯游戏身份验证文字",
	}
	key, ok := rs.BestMatch(683, tags)
	if !ok || key != "设备脸——和平精英" {
		t.Fatalf("BestMatch = (%q, %v), 期望 (设备脸——和平精英, true)", key, ok)
	}
}

// 例 4：27 个 tag 只检出 7 个 -> 走模糊匹配，校验打分明细
func TestFuzzyDetail(t *testing.T) {
	rs := loadTestRules(t)
	tags := []string{
		"王者荣耀_上号阶段7_对抗路标识", "王者荣耀_上号阶段7_射手标识",
		"王者荣耀_上号阶段7_局内交流标识", "王者荣耀_上号阶段7_局内商店",
		"王者荣耀_上号阶段7_局内地图", "王者荣耀_上号阶段7_局内语音",
		"王者荣耀_上号阶段7_帮抢标识",
	}
	want := []Detail{{
		Key: "上号阶段7——王者荣耀", Hit: 7, Total: 27,
		Coverage: 0.259, Prec: 1.0, Score: 0.412, Exact: false,
	}}
	if got := rs.MatchDetail(443, tags); !reflect.DeepEqual(got, want) {
		t.Fatalf("MatchDetail = %+v, 期望 %+v", got, want)
	}
	// 全库反查与未登记 id 结果一致
	if got := rs.MatchKeys(nil, tags); !reflect.DeepEqual(got, []string{"上号阶段7——王者荣耀"}) {
		t.Fatalf("MatchKeys(nil) = %v", got)
	}
	if key, _ := rs.BestMatch(999, tags); key != "上号阶段7——王者荣耀" {
		t.Fatalf("BestMatch(999) = %q", key)
	}
}

// 例 4b：只检出 2 个，覆盖率极低但 prec=100% -> 按 prec 入围
func TestLowCoverageHighPrec(t *testing.T) {
	rs := loadTestRules(t)
	tags := []string{"王者荣耀_上号阶段7_对抗路标识", "王者荣耀_上号阶段7_射手标识"}
	want := []Detail{{
		Key: "上号阶段7——王者荣耀", Hit: 2, Total: 27,
		Coverage: 0.074, Prec: 1.0, Score: 0.138, Exact: false,
	}}
	if got := rs.MatchDetail(443, tags); !reflect.DeepEqual(got, want) {
		t.Fatalf("MatchDetail = %+v, 期望 %+v", got, want)
	}
}

// 例 5 / 5b：顶层通用规则；1 个独占 tag 走模糊，2 个 tag 全中即精确
func TestGenericRule(t *testing.T) {
	rs := loadTestRules(t)
	if key, _ := rs.BestMatch(443, []string{"通用游戏_上号阶段3_二维码显示"}); key != "上号阶段3" {
		t.Fatalf("例5 BestMatch = %q, 期望 上号阶段3", key)
	}
	tags := []string{"通用游戏_上号阶段3_二维码显示", "通用游戏_上号阶段3_授权登录"}
	want := []Detail{{Key: "上号阶段3", Hit: 2, Total: 2, Coverage: 1, Prec: 1, Score: 1, Exact: true}}
	if got := rs.MatchDetail(443, tags); !reflect.DeepEqual(got, want) {
		t.Fatalf("例5b MatchDetail = %+v, 期望 %+v", got, want)
	}
}

// 例 6 + 名称前缀对齐："三角洲行动" -> 规则库里的 "三角洲行动手游"
func TestPrefixAlign(t *testing.T) {
	rs := loadTestRules(t)
	tags := []string{"三角洲行动_上号阶段2_QQ登录", "三角洲行动_上号阶段2_微信登录"}
	want := []Detail{{
		Key: "上号阶段2——三角洲行动手游", Hit: 2, Total: 6,
		Coverage: 0.333, Prec: 1.0, Score: 0.5, Exact: false,
	}}
	// id 1755 未登记 -> 全库反查
	if got := rs.MatchDetail(1755, tags); !reflect.DeepEqual(got, want) {
		t.Fatalf("MatchDetail(1755) = %+v, 期望 %+v", got, want)
	}
	// 名称前缀对齐 -> 严格作用域，同样命中
	if key, _ := rs.BestMatch("三角洲行动", tags); key != "上号阶段2——三角洲行动手游" {
		t.Fatalf("BestMatch(三角洲行动) = %q", key)
	}
}

// 例 7：game 未知时（nil / 未登记 id）行为完全一致；并列名次按 JSON 顺序取先出现的
func TestUnknownGameDeterministic(t *testing.T) {
	rs := loadTestRules(t)
	tags := []string{"腾讯健康系统_人脸验证窗口", "腾讯健康系统_人脸倒计时"}
	for _, game := range []any{nil, 9999, "9999"} {
		key, ok := rs.BestMatch(game, tags)
		if !ok || key != "三分钟人脸——和平精英" {
			t.Fatalf("BestMatch(%v) = (%q, %v), 期望 (三分钟人脸——和平精英, true)", game, key, ok)
		}
	}
}

// 例 8：单标签误判防护
func TestUniqueSingle(t *testing.T) {
	rs := loadTestRules(t)
	shared := []string{"腾讯健康系统_人脸验证窗口"}    // 共用 tag，同时属于多条规则
	unique := []string{"通用游戏_上号阶段3_二维码显示"} // 独占 tag

	if key, ok := rs.BestMatch(683, shared); ok {
		t.Fatalf("共用 tag 应不认，实际 = %q", key)
	}
	if key, _ := rs.BestMatch(683, shared, WithUniqueSingle(false)); key != "三分钟人脸——和平精英" {
		t.Fatalf("关闭防护 BestMatch = %q, 期望 三分钟人脸——和平精英", key)
	}
	if key, _ := rs.BestMatch(683, unique); key != "上号阶段3" {
		t.Fatalf("独占 tag BestMatch = %q, 期望 上号阶段3", key)
	}
}

// 例 8 后半：TagOwners 共用 tag 统计
func TestTagOwners(t *testing.T) {
	rs := loadTestRules(t)
	owners := rs.TagOwners()
	sharedCount := 0
	for _, keys := range owners {
		if len(keys) > 1 {
			sharedCount++
		}
	}
	if sharedCount != 9 {
		t.Fatalf("共用 tag 数 = %d, 期望 9", sharedCount)
	}
	want := []string{"三分钟人脸——和平精英", "普通脸——和平精英", "循环脸/三分钟脸——和平精英", "腾讯健康系统"}
	if got := owners["腾讯健康系统_开始验证"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("owners[腾讯健康系统_开始验证] = %v, 期望 %v", got, want)
	}
}

func TestSplitKey(t *testing.T) {
	if rule, game, ok := SplitKey("普通脸——和平精英"); !ok || rule != "普通脸" || game != "和平精英" {
		t.Fatalf("SplitKey = (%q, %q, %v)", rule, game, ok)
	}
	if rule, game, ok := SplitKey("全游戏通用_上号阶段2"); ok || rule != "全游戏通用_上号阶段2" || game != "" {
		t.Fatalf("SplitKey = (%q, %q, %v)", rule, game, ok)
	}
}

func TestLoadRulesErrors(t *testing.T) {
	if _, err := LoadRules([]byte(`[1,2]`)); err == nil {
		t.Fatal("顶层非对象应报错")
	}
	if _, err := LoadRules([]byte(`{"游戏": {"阶段": {"$bad": {"tags": []}}}}`)); err == nil {
		t.Fatal("未知操作符应报错")
	}
}

func TestVersion(t *testing.T) {
	if Version == "" {
		t.Fatal("Version 不能为空")
	}
}
