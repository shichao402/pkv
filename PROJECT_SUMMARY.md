# PKV 项目摘要

## 当前定位

PKV 是一个围绕 Bitwarden Password Manager 组织的个人密钥与配置落地工具。它以 Bitwarden folder 为组织单位，把一个 folder 内的 SSH Key、`pkv.env` Secure Note 和普通 Secure Note 同步到本机或当前项目目录。

## 入口模型

PKV 现在采用 TUI + CLI 双入口：

- `pkv`：在交互式终端中默认进入 TUI
- `pkv tui`：显式启动 TUI
- `pkv <command>`：直接执行 CLI 命令，适合脚本和自动化
- `PKV_NO_TUI=1 pkv ...`：强制禁用默认 TUI
- `TERM=dumb` 或任一 std fd 不是 TTY 时自动走 CLI

旧 readline REPL、`pkv>` prompt 和 folder-first 简写/转译语法已经移除。

## 主要 CLI 命令

```bash
pkv list [folder]
pkv get <folder> <ssh|env|note|all>
pkv add <folder> <ssh|env|note>
pkv edit <folder> <env|note> [name-or-id]
pkv remove <folder> <ssh|env|note> [id...]
pkv clean <folder> <ssh|env|note>
pkv unlock
pkv update
```

## TUI 能力

TUI 支持：

- 浏览 Bitwarden folders
- 查看 folder 下的 SSH / Env / Notes 资源
- 添加 SSH key
- 编辑 env 或 note
- 删除远端资源前确认
- 清理本地产物前确认
- 解锁 Bitwarden vault
- 刷新资源列表

## 数据模型

- SSH：使用 Bitwarden 原生 SSH Key item，本地部署到 `~/.ssh/pkv_*` 并维护 `~/.ssh/config` 的 PKV 管理区块
- Env：folder 内保留名 `pkv.env` 的 Secure Note，生成 `~/.pkv/env/<folder>.json|.sh|.ps1`
- Note：除 `pkv.env` 之外的 Secure Note，按 note 名称同步到当前目录，支持相对路径并拒绝绝对路径或 `..` 逃逸
- Include：`pkv.include` 可让一个 folder 引用其他 folder 的 env/note 默认值；当前 folder 胜出，ssh 与写操作不展开 include

## 架构概览

- `cmd/`：cobra CLI 入口、entry-mode 路由、CLI 参数适配
- `internal/tui/`：Bubbletea TUI 顶层 model、视图、交互流程
- `internal/app/`：CLI 与 TUI 共享的能力层和 Reporter 接口
- `internal/bw/`：Bitwarden CLI 封装
- `internal/key/`：SSH key 解析、生成与转换
- `internal/env/`：env 产物生成
- `internal/note/`：note 同步与冲突保护
- `internal/state/`：部署状态追踪
- `internal/version/`：构建时版本注入

## 发布流程

版本号唯一来源是 `version.json`，值不带 `v` 前缀。

常规发布流程：

1. 更新 `version.json`
2. 更新 `CHANGELOG.md`
3. 提交版本变更，例如 `chore(release): bump version to v1.0.0`
4. 推送 `main`
5. CI 读取 `version.json`，自动创建 `vX.Y.Z` tag 和 GitHub Release

不要手动创建或推送 release tag；这会让 CI 误判 tag 已存在并跳过发布构建。

## 验证建议

发布前至少运行：

```bash
go test ./...
go build ./...
go vet ./...
PKV_NO_TUI=1 go run . --version
```

涉及 TUI 的变更还应手动运行 `go run .` 验证交互式入口。涉及核心 Bitwarden 行为的变更应在个人 vault 上手动覆盖 `list` / `get` / `add` / `edit` / `remove` / `unlock` 主路径。
