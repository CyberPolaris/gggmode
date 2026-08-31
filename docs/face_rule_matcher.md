# face_rule_matcher Go 版说明文档

根据检测到的画面 tags 反推规则 key（例如 `普通脸——和平精英`、`全游戏通用_上号阶段2`）。
本包是 Python 版 `face_rule_matcher.py` 的 Go 移植，**行为与 Python 版逐例对齐**（测试用例的期望值全部取自 Python 原版对同一份 rules.json 的实际运行输出）。

**规则内容由使用方自行传入**（`LoadRules`），本包不内置任何 rules.json。

---

## 一、快速上手

```go
import (
    "os"

    "github.com/CyberPolaris/gggmode"
)

func main() {
    data, _ := os.ReadFile("rules.json")     // 规则由使用方提供
    rs, err := gggmode.LoadRules(data)
    if err != nil {
        panic(err)                            // 未知操作符等问题在加载时报错
    }

    tags := []string{
        "腾讯健康系统_人脸验证窗口", "腾讯健康系统_人脸验证标记",
        "腾讯健康系统_开始验证", "腾讯健康系统_暂不验证",
    }

    // 生产流水线：只要排第一的 key
    key, ok := rs.BestMatch(683, tags)        // "普通脸——和平精英", true

    // 看全部候选
    keys := rs.MatchKeys(683, tags)           // ["普通脸——和平精英"]

    // 调阈值 / 排查误判 / 打日志：带打分明细
    details := rs.MatchDetail(683, tags)      // [{Key Hit Total Coverage Prec Score Exact}]

    _, _, _ = key, ok, keys
    _ = details
}
```

三个入口参数完全相同，候选与排序一致，只是返回的信息量不同：
`BestMatch(...)` 的 key == `MatchKeys(...)[0]` == `MatchDetail(...)[0].Key`。

## 二、匹配策略（两级）

1. **精确命中**：某个 key 的 `$in` tag 全部出现、`$nin` tag 一个没出现
   → 直接返回这些 key（按命中 tag 数降序），不再看模糊结果。
2. **模糊命中**：没有任何精确命中时，按匹配度给每个 key 打分，分数最高的排最前：
   - 覆盖率 `coverage = 命中数 / 规则要求的 tag 数`
   - 占比 `prec = 命中数 / 检测到的 tag 数`
   - `score` = 两者的调和平均（F1）；`$nin` 命中依然是硬性否决
   - 入围门槛只看 `prec` 和最少命中数，不看覆盖率——规则含 tag 再多也不吃亏

## 三、规则语义

同一层级的多个操作符之间是 **AND** 关系：

| 操作符 | 别名 | 语义 |
|--------|------|------|
| `$in` | `$contains` | 列出的 tag 需要出现（精确=全部；模糊=命中越多分越高） |
| `$nin` | `$not_contains` | 列出的 tag 一个都不能出现（硬性否决） |
| `$must` | — | 锚点 tag：必须全部出现，缺一个整条规则不成立（精确/模糊都生效），用来把共用 tag 的规则区分开 |
| `$or` | — | 子分支取匹配度最高的一支（`$in1`、`$in2`… 数字后缀只是避免 JSON key 重复） |
| `$and` | — | 子项全部计入 |

### JSON 结构（兼容两种写法）

```jsonc
{
  "和平精英": {                      // 游戏名 -> 阶段名 -> 规则
    "game_id": 683,                  // 保留字段：id -> 游戏名反查用，不参与匹配
    "普通脸": {
      "$in":  { "tags": ["...", "..."] },
      "$nin": { "tags": ["..."] }
    }
  },
  "全游戏通用_上号阶段2": {           // 顶层key -> 规则（通用规则）
    "$in": { "tags": ["..."] }
  }
}
```

- value 的 key **全部以 `$` 开头**即视为规则（按结构识别，与命名前缀无关）
- 值为空 `{}` 或只有 `game_id` 的游戏自动跳过；非对象的顶层值忽略

## 四、game 参数

`game` 类型为 `any`，支持：

| 传法 | 行为 |
|------|------|
| 已登记的 id（int / 纯数字字符串）或名称 | 候选 = 该游戏的专属规则 + 顶层通用规则（严格作用域，不跨游戏兜底） |
| `nil` / 未登记的 id / 未登记的名称 | 行为完全一致：先只看顶层通用规则，"全中"才就地返回（游戏未知时不冒认游戏名）；否则退回全库反查 |

- id 按 JSON 里各游戏的 `game_id` 字段反查，增删游戏、改 id 只需更新 JSON
- 名称对不齐时做一次前缀对齐（如 `三角洲行动` → 规则库里的 `三角洲行动手游`）
- 传了 id 却命中别的游戏（如传 999 得到 `上号阶段7——王者荣耀`），说明 id 和画面对不上

### 返回 key 的统一格式

- 游戏专属规则：`规则名——游戏名`（连接符是 `KeySep = "——"`，规则名里请勿使用）
- 顶层通用规则：原样返回顶层 key
- 拆开用 `SplitKey(key)`，返回 `(规则名, 游戏名, hasGame)`

## 五、匹配选项（函数式选项）

| 选项 | 默认 | 说明 |
|------|------|------|
| `WithFuzzy(bool)` | `true` | 是否允许模糊命中 |
| `WithMinHits(int)` | `2` | 模糊命中至少对上的 tag 数（自动收缩到不超过实际检出数） |
| `WithMinPrec(float64)` | `0.5` | 模糊入围门槛 = 命中数 / 检出 tag 数 |
| `WithIncludeGlobal(bool)` | `true` | 指定 game 时是否连同顶层通用规则一起匹配 |
| `WithExactFirst(bool)` | `true` | 精确命中是否提第一梯队；`false` = 纯模糊模式（对比实验用） |
| `WithUniqueSingle(bool)` | `true` | 单标签误判防护，见下节 |

```go
key, ok := rs.BestMatch(game, tags, gggmode.WithMinPrec(0.6), gggmode.WithUniqueSingle(false))
```

## 六、单标签误判防护

只检出 1 个 tag 等证据不足时，`prec` 恒为 1.0、`MinHits` 又自动降到 1，两道门槛失效；
而共用 tag（如 `人脸验证窗口` 同时属于多条规则）只能靠 JSON 顺序随机挑一条——这就是误判来源。防护手段：

1. **`WithUniqueSingle(true)`（默认开）**：证据不足（hit < MinHits）的模糊命中，只有对上的 tag 在候选池内"只属于这一条规则"才认；共用 tag 一律不认，返回未命中，宁可等下一帧
2. **规则加 `$must` 锚点**：让每条规则至少有一个独占的必要 tag
3. **`TagOwners()`** 列出每个 tag 被哪些规则共用，用来审查规则库、决定加哪些锚点
4. **流水线层面**：连续 2~3 帧的 tag 取并集再匹配，或同一 key 连续出现 N 次才采纳

## 七、API 一览

```go
func LoadRules(data []byte) (*RuleSet, error)          // 编译规则库（保持 JSON 键顺序）
func SplitKey(key string) (rule, game string, hasGame bool)

func (rs *RuleSet) MatchDetail(game any, tags []string, opts ...Option) []Detail
func (rs *RuleSet) MatchKeys(game any, tags []string, opts ...Option) []string
func (rs *RuleSet) BestMatch(game any, tags []string, opts ...Option) (string, bool)
func (rs *RuleSet) GameIDMap() map[string]int          // {游戏名: game_id}
func (rs *RuleSet) TagOwners() map[string][]string     // {tag: [拥有它的规则 key...]}

type Detail struct {
    Key      string  `json:"key"`
    Hit      int     `json:"hit"`      // 对上的 tag 数
    Total    int     `json:"total"`    // 规则要求的 tag 总数
    Coverage float64 `json:"coverage"` // hit/total，三位小数
    Prec     float64 `json:"prec"`     // hit/检出tag数，三位小数
    Score    float64 `json:"score"`    // F1，三位小数
    Exact    bool    `json:"exact"`    // hit == total
}
```

## 八、与 Python 版的对应关系与差异

| Python | Go |
|--------|-----|
| `match_detail(rules, game, tags, **kw)` | `rs.MatchDetail(game, tags, opts...)` |
| `match_keys(...)` | `rs.MatchKeys(...)` |
| `best_match(...)` 返回 `None` | `rs.BestMatch(...)` 返回 `("", false)` |
| `game_id_map(rules)` | `rs.GameIDMap()` |
| `tag_owners(rules)` | `rs.TagOwners()` |
| `split_key(key)` 返回 `(rule, None)` | `SplitKey(key)` 返回 `(rule, "", false)` |
| kwargs 默认值 | `With*` 函数式选项，默认值相同 |

刻意保留的行为差异（都是更严格/更安全的方向）：

1. **未知操作符在 `LoadRules` 时报错**，Python 版在匹配时才抛 `ValueError`。规则写错能更早暴露。
2. **保序解析 JSON**：`encoding/json` 的 map 不保序，本包用自定义解析器保持 JSON 键顺序，
   保证打分并列时的排位与 Python 版（dict 插入序）一致、可复现。
3. `game` 传入不支持的类型（非 nil/字符串/整数）按"game 未知"处理，Python 版此处行为未定义。

## 九、测试

```bash
go test ./...
```

`matcher_test.go` 覆盖 Python 原版 `__main__` 里的例 1~例 8（精确命中、`$nin` 否决、
`$or` 分支、模糊打分明细、前缀对齐、game 未知路径的确定性、单标签防护、共用 tag 统计），
期望值取自 Python 版对 `testdata/rules.json` 的实际运行输出。
