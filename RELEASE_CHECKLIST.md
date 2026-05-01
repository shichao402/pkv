# PKV 发布流程

PKV 的版本与构建契约见 `.codebuddy/rules/dec-cli-release-rules.mdc`。
本文件只讲"按下按钮之前要做什么、按下之后会发生什么"。

## 权威来源

- 版本号唯一来源：`version.json`（不带 `v` 前缀，例如 `0.5.0`）
- 标签与 Release：**由 CI 创建**，由 `.github/workflows/auto-release.yml` 负责
- 触发条件：`push` 到 `main` 分支

CI 的逻辑（摘自 `auto-release.yml`）：

1. 读取 `version.json` 得到 `TAG=v${version}`
2. 如果 `refs/tags/${TAG}` 已经存在 → `should_release=false`，跳过 build 和 release（只跑 ~9 秒，什么也不产出）
3. 如果不存在 → 交叉编译 6 个平台（linux/darwin/windows × amd64/arm64），生成 `checksums.sha256`，通过 `gh release create ${TAG}` 同时创建 tag 和 Release

因此只要 `version.json` 不变，推多少次 `main` 都不会触发新 Release。

## 常规发布步骤

发布前检查：

- [ ] `go vet ./...` 通过
- [ ] `go build ./...` 通过
- [ ] 手动用自己的 Bitwarden 库跑一遍核心命令（`list` / `get` / `add` / `edit` / `remove` / `unlock`）
- [ ] 没有硬编码凭证、邮箱、个人路径
- [ ] README.md 已反映本次变更（命令/flag/行为）
- [ ] CHANGELOG.md 已新增 `## [vX.Y.Z] - YYYY-MM-DD` 段落

发布动作：

```bash
# 1. 改版本（例：0.5.0 → 0.5.1）
#    编辑 version.json，把 "version" 改成新值（不带 v）

# 2. 写 CHANGELOG.md
#    在 ## [Unreleased] 下面新增一节：
#    ## [v0.5.1] - 2026-05-01
#    ### Added / Changed / Fixed ...

# 3. 单独提交版本变更，便于溯源
git add version.json CHANGELOG.md
git commit -m "chore(release): bump version to v0.5.1"

# 4. 推 main —— 就这一步
git push origin main
```

完事之后：

```bash
# 监控 CI
gh run list --workflow=auto-release.yml --limit 1
gh run watch <run-id> --exit-status

# 验证产物
gh release view v0.5.1
```

一个成功的 Release 包含 7 个 asset：

- `pkv_darwin_amd64`
- `pkv_darwin_arm64`
- `pkv_linux_amd64`
- `pkv_linux_arm64`
- `pkv_windows_amd64.exe`
- `pkv_windows_arm64.exe`
- `checksums.sha256`

## ⚠️ 不要做的事

**不要手动打 tag 或推 tag。**

```bash
# ❌ 不要做
git tag -a v0.5.1 -m "release"
git push origin v0.5.1
```

原因：CI 判断"tag 已存在"时会走 skip 分支，导致不构建、不上传、没有 Release。
如果你手推了 tag，表现是：

- `gh run list` 看到最近一次 auto-release 运行时长 ~9 秒（正常 ~60-90 秒）
- `gh release view vX.Y.Z` 返回 `release not found`

## 误推 tag 的修复流程

```bash
# 1. 删远端 tag（GitHub 上的）
git push origin :refs/tags/vX.Y.Z

# 2. 删本地 tag
git tag -d vX.Y.Z

# 3. workflow 只监听 push: main，所以必须再推一次 main 才能重新触发
#    最简单的办法：推一个空提交
git commit --allow-empty -m "ci: retrigger auto-release for vX.Y.Z"
git push origin main

# 4. 确认
gh run list --workflow=auto-release.yml --limit 1
gh release view vX.Y.Z
```

这是在发布 v0.5.0 时实际踩过的坑，记在这里防止换台机器再犯。

## 版本号选择

遵循 SemVer（见 `.codebuddy/rules/dec-cli-release-rules.mdc` § 1.2）：

- PATCH：向后兼容的修复
- MINOR：向后兼容的新功能（新增 CLI 命令、flag、子命令通常都属于这一级）
- MAJOR：破坏性变更（命令语义、flag 语义或配置格式改动导致旧用法失效）

## 首次发布的配置

如果有一天要把这个 workflow 搬到新仓库，一次性准备：

- 仓库 Settings → Actions → General → Workflow permissions：勾 `Read and write permissions`（让 `GITHUB_TOKEN` 能创建 Release）
- 确认 `main` 是默认分支，`auto-release.yml` 在 `.github/workflows/` 下
- `version.json` 初始值即首个 Release 版本号
