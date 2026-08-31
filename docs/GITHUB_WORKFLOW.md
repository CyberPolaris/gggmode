# GitHub 使用规范与操作记录

本文档记录 gggmode 包的开发环境、GitHub 协作规范，以及环境搭建的操作记录。

---

## 一、开发环境（2026-08-31 梳理）

| 组件 | 版本 | 安装位置 | 说明 |
|------|------|----------|------|
| Go | 1.27.0 | `C:\Program Files\Go` | 由 1.25.4 升级（原在 `D:\Program Files\Go`，官方 MSI 升级后迁到 C 盘） |
| Git for Windows | 2.55.0.windows.5 | `D:\Program Files\Git` | 由 2.54.0 通过 `git update-git-for-windows` 升级 |
| GitHub CLI (gh) | 最新 | winget 安装 | 用于命令行创建仓库、管理 PR/issue |
| GOPATH | — | `C:\Users\peng\go` | 未改动 |
| GOPROXY | 默认 | `https://proxy.golang.org,direct` | 国内建议改为 `go env -w GOPROXY=https://goproxy.cn,direct` |

Git 全局身份：`CyberPolaris <m13507262368@163.com>`

## 二、GitHub 协作规范

### 仓库

- 模块路径：`github.com/CyberPolaris/gggmode`（go.mod 与 GitHub 仓库名必须一致）
- 默认分支：`main`
- 换行符策略：`core.autocrlf=input`（仓库内统一 LF）

### 分支命名

| 前缀 | 用途 | 示例 |
|------|------|------|
| `feat/` | 新功能 | `feat/parser` |
| `fix/` | 修 bug | `fix/nil-pointer` |
| `docs/` | 文档 | `docs/api-guide` |
| `refactor/` | 重构 | `refactor/split-config` |
| `chore/` | 构建/依赖等杂项 | `chore/bump-deps` |

`main` 分支不直接提交，一律通过分支 + Pull Request 合入。

### Commit 规范（Conventional Commits）

格式：`<type>(<scope>): <描述>`

- type 取值：`feat` / `fix` / `docs` / `refactor` / `test` / `chore` / `perf`
- 描述用祈使句、首字母小写、不加句号，例如：
  - `feat: add config parser`
  - `fix(parser): handle empty input`
- 破坏性变更在 type 后加 `!`，如 `feat!: drop go1.25 support`

### Pull Request 流程

1. 从 `main` 拉分支开发：`git switch -c feat/xxx`
2. 提交前本地自检：`go build ./... && go vet ./... && go test ./...`
3. 推送并创建 PR：`git push -u origin feat/xxx && gh pr create`
4. PR 描述写清楚"改了什么、为什么改"；关联 issue 用 `Closes #N`
5. CI 通过后合并，优先使用 **Squash and merge** 保持 main 历史整洁
6. 合并后删除远端分支

### 版本发布

- 遵循 [语义化版本](https://semver.org/lang/zh-CN/)：`vMAJOR.MINOR.PATCH`
- 打 tag 发布：`git tag v0.1.0 && git push origin v0.1.0`
- Go 模块代理会自动收录 tag，用户即可 `go get github.com/CyberPolaris/gggmode@v0.1.0`
- v1 之前 API 允许变动；升到 v2+ 时模块路径需加 `/v2` 后缀（Go 模块规则）

## 三、操作记录

### 2026-08-31 环境搭建

1. **升级 Go**：1.25.4 → 1.27.0
   - 官方 MSI 静默安装（SHA256 已校验：`728e9318...`）
   - 安装位置从 `D:\Program Files\Go` 变为 `C:\Program Files\Go`，系统 PATH 已自动更新
2. **升级 Git**：2.54.0 → 2.55.0.windows.5，命令 `git update-git-for-windows -y`
3. **安装 GitHub CLI**：`winget install --id GitHub.cli`
4. **初始化项目**：
   ```bash
   go mod init github.com/CyberPolaris/gggmode
   git init -b main
   git config core.autocrlf input
   ```
5. **创建骨架文件**：`gggmode.go`、`gggmode_test.go`、`.gitignore`、`README.md`、本文档

### 待办

- [ ] `gh auth login` 登录 GitHub 账号（需人工在浏览器完成授权）
- [ ] 确定包的实际功能，补充 `gggmode.go` 与 README 描述
- [ ] 在 GitHub 创建仓库并推送：`gh repo create CyberPolaris/gggmode --public --source . --push`
- [ ] 选择 License（建议 MIT 或 Apache-2.0）
- [ ] （可选）配置 GitHub Actions CI：build + vet + test
- [ ] （可选，国内网络）`go env -w GOPROXY=https://goproxy.cn,direct`
