# gggmode

> 根据检测到的画面 tags 反推规则 key，两级匹配：精确命中优先，无精确命中时按 F1 打分模糊匹配。Python 版 face_rule_matcher 的 Go 移植，行为逐例对齐。

[![Go Version](https://img.shields.io/badge/go-1.27-blue)](https://go.dev/)

## 安装

```bash
go get github.com/CyberPolaris/gggmode
```

## 使用

规则内容（rules.json）由使用方自行传入，本包不内置任何规则：

```go
data, _ := os.ReadFile("rules.json")
rs, err := gggmode.LoadRules(data)
if err != nil {
    // 未知操作符等规则问题在加载时报错
}

key, ok := rs.BestMatch(683, tags)   // 生产流水线：只取排第一的 key
keys := rs.MatchKeys(683, tags)      // 全部候选 key
details := rs.MatchDetail(683, tags) // 带打分明细，调阈值/排查误判用
```

完整说明（规则语义、game 参数、匹配选项、误判防护、与 Python 版的差异）见
[docs/face_rule_matcher.md](docs/face_rule_matcher.md)。

## 开发

开发环境、GitHub 协作规范与操作记录见 [docs/GITHUB_WORKFLOW.md](docs/GITHUB_WORKFLOW.md)。

```bash
go build ./...   # 编译
go test ./...    # 测试
go vet ./...     # 静态检查
```

## License

待定（建议 MIT 或 Apache-2.0，创建 GitHub 仓库时添加）。
