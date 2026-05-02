# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v0.5.2] - 2026-05-02

### Changed
- `.golangci.yml` 升级到 golangci-lint v2 schema（`linters.exclusions.paths/rules`），CI 使用 `golangci/golangci-lint-action@v7` + `version: v2.11`，解决 Go 1.25 下旧版本 linter 拒绝分析的问题
- 多处数据结构读循环改为索引 + 指针访问（`cmd/resources.go`、`internal/state/state.go`），避免每次迭代复制 SSH/Env/Note 条目（136–152 字节）

### Fixed
- 修正 `cmd/resources.go` 里的 `Cancelled → Canceled`，以及若干 `usage: ...` 错误信息尾部标点、`fmt.Errorf` 首字母等 staticcheck ST1005/QF1001/QF1006 问题
- `internal/securenote/editor.go` 的 `OpenEditor`：不再 append 到 `parts[1:]` 的底层数组（appendAssign），并在写临时文件失败时向调用方回传 `tmpFile.Close()` 的错误
- `cmd/update.go`、`cmd/shell.go` 等处的 `defer f.Close()` / `defer rl.Close()` 以显式忽略形式关闭，满足 errcheck
- 清理 `internal/key/parse_test.go` 未使用的测试常量，移除 `internal/key/bitwarden.go` 多余的 `[]byte()` 转换

## [v0.5.1] - 2026-05-01

### Docs
- 重写 `RELEASE_CHECKLIST.md`，明确 PKV 的发布流程是"改 `version.json` → 推 main"，tag 由 `.github/workflows/auto-release.yml` 创建；不要手动 `git tag` / `git push origin vX.Y.Z`（会让 CI 判定 tag 已存在从而跳过构建）。同时记录手推 tag 后的恢复流程（删远端 tag + 空 commit 重新触发）
- `CONTRIBUTING.md` 的 Release Process 小节与新流程对齐，并回链到 `RELEASE_CHECKLIST.md`

## [v0.5.0] - 2026-05-01

### Added
- 新增 `pkv unlock` 子命令，用于解锁 Bitwarden 并把 session 打到 stdout，方便 `export BW_SESSION="$(pkv unlock)"` 或 `eval "$(pkv unlock --export)"` 在脚本里复用
- 交互模式里支持 `pkv> unlock` 预热 session，不回显 token

### Changed
- 同一个 `pkv` 进程内会复用最近一次成功的 `bw sync` 结果，连续只读命令不再重复触发远端同步；一旦本轮出现写操作，会自动失效缓存
- PKV 启动时先探测 `bw --version`，若 `bw` 缺失或输出异常，在进入 Bitwarden 认证前就直接报错
- README 重写「自动化与脚本化」一节，以 `pkv unlock` 为推荐路径，并说明为什么 PKV 不提供 `--master-pass` 之类的入口（保持"PKV 永不接触主密码"作为代码层不变量）

## [v0.4.2] - 2026-04-16

### Added
- `pkv get <folder> note` 会在真正落盘前聚合冲突并一次性返回完整问题清单，失败时保证本地零副作用
- 新增对 Note 冲突预检、同路径冲突、文件/目录形态冲突、已追踪文件重命名进子目录等场景的测试覆盖

### Changed
- `pkv get <folder> note` 继续保持“Secure Note 名称即目标相对路径”的简单模型，不引入额外的 per-note 策略字段
- README 补充了 Note 路径与冲突处理说明，明确当前推荐用法

## [v0.4.1] - 2026-04-07

### Added
- `pkv get <folder> note` 现在支持按 Secure Note 名称里的相对路径创建嵌套目录，例如 `lyra/test/note`
- 新增 Note 嵌套路径、安全路径校验以及清理空目录的测试覆盖

### Changed
- Note 同步会拒绝绝对路径和 `..` 路径，避免写出当前目标目录之外
- 交互模式移除 `dev get env` 这类 folder-action-first 语法，只保留标准命令和 `dev env` 这种简写
- README 更新了新的 note 路径规则与交互命令示例

## [v0.4.0] - 2026-04-06

### Added
- 交互模式支持历史记录、方向键切换与 `Ctrl+R` 反向搜索
- 交互模式支持 `dev env`、`prod note add ...` 这类 folder 在前的资源简写
- 新增 `PKV_DEBUG=1` 脱敏诊断日志能力，覆盖 shell、Bitwarden 会话与 env 产物部署路径
- 新增导出 `BW_SESSION` 复用、失效重试与交互 shell 翻译相关测试

### Changed
- Bitwarden 客户端会先校验并复用导出的 `BW_SESSION`，失效时自动回退到交互式解锁流程
- `bw` 调用统一改为显式 `--nointeraction` / `--session` 形式，错误上下文与调试信息更清晰
- README 补充了交互模式快捷操作、失效 session 排查和调试日志说明

## [v0.3.0] - 2026-04-06

### Added
- `pkv list [folder]` 用于列出 Bitwarden folders 或单个 folder 内的资源
- 统一的 `pkv get|add|edit|remove|clean <folder> <ssh|env|note>` 资源命令模型
- 直接执行 `pkv` 进入交互模式，并在同一进程内复用 `BW_SESSION`
- `pkv get <folder> env` 生成 `~/.pkv/env/<folder>.json|.sh|.ps1` 三类 env 产物
- SSH 与 Note 同步的远端对齐能力，包括远端删除清理与重命名跟随
- 基于内容哈希和目标目录的 Note 状态追踪与本地冲突保护
- 新增 shell 解析、env 产物、note 对齐行为相关测试

### Changed
- `pkv.env` 成为 folder 级 env 数据的保留 Secure Note 名称，兼容历史 `pkv_type=env` 标记
- `pkv get <folder> env` 改为只生成本地文件，不再持久写入系统环境变量
- `pkv get <folder> note` 改为按 `folder + targetDir + itemID` 对齐，并拒绝覆盖本地已修改文件
- SSH 部署会根据状态追踪重建 `known_hosts`，并回收远端已删除的本地 key
- Secure Note 创建与更新流程改为返回结构化 item ID，便于状态管理与后续操作
- 发布流程改为以 `version.json` 为单一版本来源，避免推送后再次自动改写版本号

### Removed
- 旧的 `pkv ssh`、`pkv env`、`pkv note` 命令层级
- 直接修改系统持久环境变量及相关 snapshot / 平台适配实现

## [v0.2.3] - 2026-03-29

### Changed
- Replaced GitHub API calls with HTTP redirect-based version detection in `install.sh`, `install.ps1`, and `pkv update`
- Eliminates API rate limiting issues for unauthenticated users

## [v0.2.2] - 2026-03-29

### Added
- `pkv env <folder>` - Deploy environment variables from Bitwarden Secure Notes
- `pkv env <folder> clean` - Remove deployed environment variables
- Supports KEY=VALUE, export KEY=VALUE, comments, and quoted values
- On Linux/macOS, writes to `~/.pkv/env.sh` and auto-sources from shell rc files
- On Windows, sets persistent User environment variables via PowerShell

### Changed
- Introduced `version.json` as the single source of truth for version numbers

## [v0.1.0] - 2026-03-28

### Added
- Initial release of PKV
- `pkv ssh <folder>` - Deploy SSH keys from Bitwarden folder with automatic config generation
- `pkv ssh <folder> clean` - Remove deployed SSH keys and configuration
- `pkv note <folder>` - Sync Bitwarden Secure Notes to current directory as files
- `pkv note <folder> clean` - Remove synced note files
- `pkv update` - Self-update to latest version from GitHub Releases
- `pkv --version` - Display version information
- Automatic `ssh-keyscan` for `known_hosts` management
- Installation script for one-command setup
- Support for Linux and macOS, amd64 and arm64 architectures
