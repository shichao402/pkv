# PKV 项目发布准备完成

## 📦 项目结构

```
pkv/
├── .github/workflows/
│   └── release.yml              # GitHub Actions 自动构建和发布
├── cmd/
│   ├── root.go                  # 根命令 & 版本管理
│   ├── ssh.go                   # SSH 密钥部署命令
│   ├── note.go                  # Note 同步命令
│   └── update.go                # 自更新命令
├── internal/
│   ├── bw/
│   │   ├── client.go            # Bitwarden CLI 包装
│   │   └── types/types.go       # 数据类型定义
│   ├── ssh/
│   │   ├── deployer.go          # SSH 部署逻辑
│   │   ├── config.go            # ~/.ssh/config 管理
│   │   └── known_hosts.go       # ~/.ssh/known_hosts 管理
│   ├── note/
│   │   └── syncer.go            # Note 文件同步
│   ├── state/
│   │   └── state.go             # 部署状态追踪 (~/.pkv/state.json)
│   └── version/
│       └── version.go           # 版本注入
├── main.go                      # 程序入口
├── go.mod                       # Go 模块定义
├── Makefile                     # 构建脚本
├── install.sh                   # 一键安装脚本
├── README.md                    # 完整用户文档
├── CHANGELOG.md                 # 变更日志
├── CONTRIBUTING.md              # 贡献指南
├── RELEASE_CHECKLIST.md         # 发布清单
├── LICENSE                      # MIT 许可证
└── .gitignore                   # Git 忽略规则
```

## ✅ 已完成的工作

### 核心功能
- ✅ SSH 密钥自动部署 (`pkv ssh <folder>`)
- ✅ SSH 配置自动生成 (支持 host:port 解析)
- ✅ known_hosts 自动扫描
- ✅ SSH 密钥精确清理 (`pkv ssh <folder> clean`)
- ✅ Note 同步导出 (`pkv note <folder>`)
- ✅ Note 精确清理 (`pkv note <folder> clean`)
- ✅ 自更新功能 (`pkv update`)
- ✅ 版本管理

### 工程化
- ✅ 脱敏审计完成（无硬编码凭证）
- ✅ GitHub Actions 自动化构建流程
- ✅ 跨平台交叉编译（4 个平台）
- ✅ 一键安装脚本（支持自动 PATH 配置）
- ✅ SHA256 校验和生成

### 文档
- ✅ 完整 README（中文）
- ✅ 快速开始指南
- ✅ 命令参考
- ✅ 故障排除
- ✅ 安全考虑说明
- ✅ 贡献指南
- ✅ 发布清单

### 安全
- ✅ 无硬编码密钥或密码
- ✅ 凭证仅在运行时获取
- ✅ 文件权限正确设置 (0600/0644)
- ✅ 使用标记注释隔离管理的配置
- ✅ 状态文件只记录元数据

## 🚀 发布步骤

### 1. 初始化本地仓库

```bash
cd ~/workspace/PKV
git init
git add -A
git commit -m "Initial commit: PKV v0.1.0

- SSH key deployment from Bitwarden
- Automatic ~/.ssh/config generation
- SSH key cleanup with known_hosts management
- Note synchronization
- Self-update functionality
- Cross-platform support (Linux, macOS, amd64, arm64)"
```

### 2. 创建 GitHub 仓库

在 GitHub 创建新仓库：
- 仓库名: `pkv`
- 所有者: `shichao402`
- 描述: "Personal Key Vault - SSH key and config manager from Bitwarden"
- 公开仓库
- 初始化时不创建任何文件（本地已有）

### 3. 连接远程

```bash
git remote add origin https://github.com/shichao402/pkv.git
git branch -M main
git push -u origin main
```

### 4. 创建第一个 Release

```bash
git tag v0.1.0 -m "Initial release"
git push origin v0.1.0
```

GitHub Actions 会自动：
1. 检测 tag 推送
2. 为 4 个平台交叉编译
3. 生成 SHA256 校验和
4. 创建 GitHub Release
5. 上传所有二进制文件

### 5. 验证发布

访问 https://github.com/shichao402/pkv/releases，检查：
- Release v0.1.0 是否已创建
- 4 个二进制文件是否已上传
- 4 个 .sha256 文件是否已上传

## 📋 发布前最终检查

```bash
cd ~/workspace/PKV

# 1. 编译检查
make clean
make build
go vet ./...

# 2. 命令验证
./pkv --version
./pkv ssh --help
./pkv note --help
./pkv update --help

# 3. 脚本验证
bash -n install.sh

# 4. 交叉编译
make release
ls -lh dist/

# 5. 仓库准备
git status  # 应无未提交的文件（除 dist/）
```

## 🎯 用户使用流程

### 首次安装

```bash
curl -fsSL https://raw.githubusercontent.com/shichao402/pkv/main/install.sh | bash
# ✅ 自动下载最新版本并安装到 ~/.local/bin
```

### 首次使用

```bash
# 1. 准备 Bitwarden（创建 SSH Key Item）
# 2. 部署 SSH 密钥
pkv ssh LyraX
# ✅ 输入主密码 + 二次验证
# ✅ 自动扫描 known_hosts
# ✅ 完成

# 3. 直接使用 SSH
ssh github.com
```

### 自更新

```bash
pkv update
# ✅ 检查最新版本
# ✅ 自动下载并替换
```

## 📊 性能指标

- 构建时间: ~5 秒（本地编译）
- 安装大小: ~8MB（单个二进制）
- 启动时间: <100ms
- 初次部署时间: ~10-30 秒（取决于 Bitwarden vault 大小和网络）

## 🔐 安全检查清单

- ✅ 无硬编码密钥、密码、token
- ✅ 无个人信息（邮箱、用户名）
- ✅ 无 API key 或认证凭证
- ✅ Bitwarden 密钥只在运行时加载
- ✅ 文件权限正确（0600 for secrets）
- ✅ 所有依赖都来自官方可信源
- ✅ 代码已审计，符合发布标准

## 📝 后续维护计划

### 版本管理
- 使用语义化版本 (SemVer)
- 更新 CHANGELOG.md
- 标签推送自动触发 Actions

### 更新流程
1. 开发特性或修复
2. 更新 CHANGELOG.md
3. 创建 tag 和 release
4. 用户运行 `pkv update` 自动更新

## 🏗 架构决策记录

### Bitwarden CLI vs SDK 分析 (2026-03)

#### 现状：PKV 完全依赖 Bitwarden CLI (`bw`)

PKV 通过 `internal/bw/client.go` 封装 `bw` CLI，涉及以下命令：

| 命令 | 用途 |
|------|------|
| `bw status` | 检查认证/锁定状态 |
| `bw login` | 交互式登录 |
| `bw unlock --raw` | 获取会话令牌 |
| `bw sync` | 同步保险库数据 |
| `bw list folders --search` | 查找文件夹 |
| `bw list items --folderid` | 获取文件夹内项目 |
| `bw create item` | 创建 SSH 密钥项 |

#### 决策：保持 CLI 方案，不接入 SDK

**原因：**

1. **功能不匹配** — Bitwarden Go SDK (`github.com/bitwarden/sdk-go`) 仅支持 Secrets Manager，不支持 Password Manager。PKV 依赖的 SSH Key (type=5)、Secure Note (type=2)、Folder 操作在 SDK 中均不存在。
2. **许可证风险** — Bitwarden SDK 采用专有许可证（2024年10月更改），限制用于 Bitwarden 产品之外的模块，PKV 使用可能违反条款。
3. **编译复杂性** — SDK 依赖 CGO 和 C 编译环境，会显著增加跨平台编译难度，且引入 Rust FFI 依赖链。
4. **CLI 是官方标准接口** — Bitwarden 未提供 Password Manager 的公开 SDK，CLI 是唯一稳定且开源的个人密码库接口。

#### 改进方向（在 CLI 框架内优化）

**1. 改进 CLI 进程管理**
- 添加 `bw` CLI 版本检查，验证最低兼容版本
- 增强错误消息，针对常见故障（网络超时、会话过期、CLI 版本不兼容）提供具体排查指引
- 考虑检测 `bw` CLI 响应格式变化，提前发现兼容性问题

**2. 会话优化**
- 缓存 `bw sync` 结果，减少重复网络调用（同一会话内多次操作时复用）
- 支持会话令牌复用：跨多条 `pkv` 命令共享 `BW_SESSION`，避免用户反复输入主密码
- 考虑引入会话超时检测，在令牌失效前主动刷新

**3. 离线模式支持**
- 支持从上次 `bw sync` 的本地缓存数据中读取（`bw` CLI 本身有本地数据）
- 在无网络时降级为只读模式，允许查看已部署的密钥状态
- 实现延迟同步策略：先使用本地数据完成操作，有网络时再同步验证

## 🎉 项目完成状态

| 组件 | 状态 |
|------|------|
| 核心功能 | ✅ 完成 |
| 自动化构建 | ✅ 完成 |
| 跨平台支持 | ✅ 完成 |
| 文档 | ✅ 完成 |
| 脱敏审计 | ✅ 完成 |
| 安全检查 | ✅ 完成 |
| 发布准备 | ✅ 完成 |

**项目已准备好发布到 GitHub！🚀**

---

## 联系方式

- GitHub: https://github.com/shichao402/pkv
- Issues: https://github.com/shichao402/pkv/issues
