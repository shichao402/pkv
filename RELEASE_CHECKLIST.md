# GitHub 发布清单

## 前置准备

- [ ] 所有代码已编译通过
- [ ] 所有脱敏检查已完成（无硬编码凭证）
- [ ] README.md 已更新
- [ ] CHANGELOG.md 已更新
- [ ] install.sh 已验证

## 发布步骤

### 1. 初始化 Git 仓库

```bash
cd ~/workspace/PKV
git init
git add -A
git commit -m "Initial commit: PKV v0.1.0"
```

### 2. 创建远程仓库

在 GitHub 上创建新仓库 `shichao402/pkv`（名称要与代码中的 module 一致）

### 3. 连接本地和远程

```bash
git remote add origin https://github.com/shichao402/pkv.git
git branch -M main
git push -u origin main
```

### 4. 创建并推送 Release 标签

```bash
# 标签名必须以 'v' 开头，GitHub Actions 根据此触发
git tag v0.1.0 -m "Initial release"
git push origin v0.1.0
```

### 5. 监控 Actions

访问 https://github.com/shichao402/pkv/actions，查看自动构建进度

预期行为：
- 4 个平台的交叉编译（darwin/amd64, darwin/arm64, linux/amd64, linux/arm64）
- 生成 SHA256 校验和
- 自动创建 Release

### 6. 验证发布

访问 https://github.com/shichao402/pkv/releases，检查：
- [ ] v0.1.0 Release 已创建
- [ ] 4 个二进制文件已上传
- [ ] 4 个 .sha256 校验和文件已上传
- [ ] Release notes 正确显示

### 7. 测试安装脚本

在干净的机器或新环境测试：

```bash
# 测试在线安装
curl -fsSL https://raw.githubusercontent.com/shichao402/pkv/main/install.sh | bash

# 验证
~/.local/bin/pkv --version
~/.local/bin/pkv update
```

## 文件检查清单

### 源代码结构
- [ ] `main.go` - 程序入口
- [ ] `cmd/` - 命令实现
  - [ ] `root.go` - 根命令
  - [ ] `ssh.go` - SSH 命令
  - [ ] `note.go` - Note 命令
  - [ ] `update.go` - 更新命令
- [ ] `internal/` - 内部包
  - [ ] `bw/` - Bitwarden 包装
  - [ ] `ssh/` - SSH 管理
  - [ ] `note/` - Note 管理
  - [ ] `state/` - 状态管理
  - [ ] `version/` - 版本信息

### 文档
- [ ] `README.md` - 主文档
- [ ] `CHANGELOG.md` - 变更日志
- [ ] `CONTRIBUTING.md` - 贡献指南
- [ ] `LICENSE` - MIT 许可证

### 配置文件
- [ ] `.gitignore` - 忽略规则
- [ ] `go.mod` - Go 模块定义
- [ ] `Makefile` - 构建脚本
- [ ] `.github/workflows/release.yml` - GitHub Actions

### 脚本
- [ ] `install.sh` - 安装脚本

## 脱敏验证

- [ ] 没有 `.env` 文件
- [ ] 没有硬编码 API keys
- [ ] 没有硬编码密码或 token
- [ ] 没有个人信息（邮箱、用户名等）
- [ ] 所有凭证都从 Bitwarden 动态获取

## 功能验证（发布前）

```bash
# 1. 本地构建
make build
./pkv --version

# 2. SSH 命令
./pkv ssh --help
./pkv ssh LyraX        # 需要 Bitwarden 访问权限

# 3. Note 命令
./pkv note --help

# 4. 更新命令
./pkv update --help

# 5. 交叉编译
make release
ls -la dist/
```

## 发布后验证

```bash
# 验证 install.sh 可访问
curl -fsSL https://raw.githubusercontent.com/shichao402/pkv/main/install.sh | head -20

# 验证 GitHub API
curl -s https://api.github.com/repos/shichao402/pkv/releases/latest | grep tag_name

# 验证二进制文件可下载
curl -fsSL -I https://github.com/shichao402/pkv/releases/download/v0.1.0/pkv_linux_amd64
```

## 持续发布流程

后续版本发布只需：

```bash
# 更新版本
# - 编辑任何源文件或文档
# - 更新 CHANGELOG.md

# 提交
git add -A
git commit -m "feat: add new feature"

# 发布
git tag v0.2.0
git push origin main v0.2.0

# GitHub Actions 自动构建并发布
```

## 常见问题

**Q: Actions 构建失败怎么办？**
A: 查看 Actions 详细日志，通常是环境配置或依赖问题。

**Q: 二进制文件下载失败？**
A: 确保 Release 已完全创建，稍等几秒后重试。

**Q: 怎么修正已发布的版本？**
A: 删除旧标签和 Release，创建新版本标签。

## 成功标志

🎉 当以下条件都满足时，发布成功：

1. ✅ 所有 4 个平台的二进制文件都在 Release 页面上
2. ✅ install.sh 可通过 curl 在线运行
3. ✅ `pkv update` 能成功检查到最新版本
4. ✅ README 在 GitHub 上正确显示
5. ✅ 新用户可以直接运行 install.sh 安装
