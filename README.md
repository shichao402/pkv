# PKV - Personal Key Vault

**PKV** 是一个命令行工具，用于从 **Bitwarden 密码库** 便捷地部署和管理 SSH 密钥及敏感配置文件。一条命令快速启用 SSH，自动配置 `~/.ssh/config` 和 `known_hosts`。

## 功能

- 🔐 **从 Bitwarden 自动部署 SSH 密钥** - 无需手动复制粘贴
- ⚡ **自动配置 SSH** - 生成完整的 `~/.ssh/config`，支持自定义端口、多主机
- 🔑 **导入 SSH 密钥到 Bitwarden** - 将本地 SSH 私钥存储到指定文件夹的 Bitwarden SSH Key Item
- 🌐 **部署环境变量** - 从 Bitwarden Note 同步 KEY=VALUE 到系统环境变量
- 📝 **同步敏感配置文件** - 将 Bitwarden Note 快速导出到当前目录
- 🧹 **精确清理** - 支持 `clean` 命令，安全移除部署的密钥和配置，不损害手动添加的内容
- 🔄 **自动更新** - `pkv update` 检查并下载最新版本
- 🌍 **跨平台** - 支持 Linux、macOS、Windows，amd64、arm64 架构

## 快速开始

### 1. 安装

**macOS / Linux：**
```bash
curl -fsSL https://raw.githubusercontent.com/shichao402/pkv/main/install.sh | bash
```

**Windows (PowerShell)：**
```powershell
irm https://raw.githubusercontent.com/shichao402/pkv/main/install.ps1 | iex
```

验证安装：
```bash
pkv --version
```

> macOS/Linux 安装到 `~/.local/bin`，Windows 安装到 `%LOCALAPPDATA%\pkv`。
> 脚本会自动检测 PATH，提示添加（如未包含）。

### 2. 准备 Bitwarden 数据

在 Bitwarden 中创建或编辑 SSH 密钥 Item，要求：

#### SSH Key Item
- **类型**：SSH Key（Bitwarden 有专门的 SSH 密钥类型）
- **名称**：任意（如 `github-key`、`server-key` 等）
- **Notes（备注）**：填写可应用的主机地址，**一行一个**
  ```
  github.com
  10.0.0.1:2222
  *.example.com
  ```

#### Env Note（环境变量）
- **类型**：Secure Note
- **名称**：任意（如 `database-creds`）
- **自定义字段**：添加一个字段，**Name** 为 `pkv_type`，**Value** 为 `env`
- **内容**：KEY=VALUE 格式，一行一个
  ```
  DB_HOST=localhost
  DB_USER=admin
  export DB_PASS="s3cret"
  # 注释会被忽略
  ```

> **重要**：`pkv env` 命令**只会处理**标记了 `pkv_type=env` 的 Secure Note，未标记的会被跳过并提示警告。这样可以避免普通文本 Note 被误当作环境变量部署。

#### Config Note（配置文件）
- **类型**：Secure Note
- **名称**：任意（如 `config.json`，会作为导出的文件名）
- **内容**：任意文本

> `pkv note` 命令会自动排除标记了 `pkv_type=env` 的 Note，其余 Note 正常同步为文件。

### 3. 部署

```bash
# 从 LyraX 文件夹部署所有 SSH 密钥
pkv ssh LyraX

# 执行过程：
# - 提示输入 Bitwarden 主密码和二次验证
# - 自动扫描所有目标主机，添加到 known_hosts
# - 生成 ~/.ssh/config 和 ~/.ssh/pkv_* 密钥文件
```

### 4. 使用

SSH 配置已自动生成，直接使用即可：
```bash
ssh github.com
ssh -p 2222 10.0.0.1
ssh user@example.com
```

### 5. 部署环境变量

```bash
# 从 credentials 文件夹部署环境变量
pkv env credentials

# 清理已部署的环境变量
pkv env credentials clean
```

> Linux/macOS 下变量写入 `~/.pkv/env.sh` 并自动 source；Windows 下设为用户级环境变量。
> 部署后需要打开新终端才能生效。

### 6. 同步配置文件

```bash
cd ~/my-project

# 从 LyraX 文件夹同步所有 Note 为文件到当前目录（自动排除 env 类型）
pkv note LyraX
# 结果：lyraXX 文件被创建到当前目录
```

### 7. 清理

```bash
# 移除所有部署的 SSH 密钥和配置
pkv ssh LyraX clean

# 移除所有同步的 note 文件
pkv note LyraX clean
```

## 命令参考

### 管理 SSH 密钥

```bash
pkv ssh <folder>                       # 从指定文件夹部署 SSH 密钥
pkv ssh <folder> list                  # 列出文件夹中的 SSH 密钥
pkv ssh <folder> add                   # 交互式导入本地 SSH 密钥到文件夹
pkv ssh <folder> remove <id> [id2]...  # 从 Bitwarden 删除指定密钥
pkv ssh <folder> clean                 # 清理本地已部署的 SSH 密钥
```

**`add` 选项**：
- `--priv` - 私钥文件路径（省略则交互式输入）
- `--pub` - 公钥内容，`ssh-rsa AAAA...` 格式（省略则从私钥自动生成）
- `--name` - 密钥在 Bitwarden 中的名称（省略则交互式输入）

**支持的私钥格式**：PEM（PKCS1/PKCS8/EC）、OpenSSH，自动转换为 OpenSSH 标准格式存储。

**例子**：
```bash
pkv ssh LyraX                  # 部署文件夹中所有密钥
pkv ssh LyraX list             # 列出密钥（显示 ID、名称、指纹、主机）
pkv ssh LyraX add --priv ~/.ssh/id_ed25519 --name "github-key"  # 导入密钥
pkv ssh LyraX remove 123e4567-e89b-12d3-a456-426614174000       # 删除密钥
pkv ssh LyraX clean            # 清理本地部署
```

### 部署环境变量

```bash
pkv env <folder>                           # 从指定文件夹部署环境变量（仅处理 pkv_type=env 的 Note）
pkv env <folder> list                      # 列出文件夹中的环境变量 Note
pkv env <folder> add --name <name> [--file <path>]  # 创建环境变量 Note
pkv env <folder> remove <id> [id2]...      # 从 Bitwarden 删除指定的环境变量 Note
pkv env <folder> edit <name-or-id>         # 用 $EDITOR 编辑环境变量 Note
pkv env <folder> clean                     # 清理已部署的环境变量
```

**`add` 选项**：
- `--name` - Note 名称（必需）
- `--file` - 从文件读取内容；省略则打开 `$EDITOR` 编写

**`edit` 参数**：
- `<name-or-id>` - 支持按名称或 ID 定位，优先按名称匹配

**要求**：Secure Note 必须设置自定义字段 `pkv_type=env`，否则会被跳过。

**例子**：
```bash
pkv env credentials            # 部署
pkv env credentials list       # 列出
pkv env credentials add --name "database" --file .env.prod  # 从文件创建
pkv env credentials edit "database"  # 编辑
pkv env credentials remove abc123def  # 删除
pkv env credentials clean      # 清理
```

### 同步配置文件

```bash
pkv note <folder>                          # 从指定文件夹导出 Note 到当前目录（排除 pkv_type=env）
pkv note <folder> list                     # 列出文件夹中的配置 Note
pkv note <folder> add --name <name> [--file <path>]  # 创建配置 Note
pkv note <folder> remove <id> [id2]...     # 从 Bitwarden 删除指定配置 Note
pkv note <folder> edit <name-or-id>        # 用 $EDITOR 编辑配置 Note
pkv note <folder> clean                    # 移除已同步的 Note 文件
```

**`add` 选项**：
- `--name` - Note 名称（必需）
- `--file` - 从文件读取内容；省略则打开 `$EDITOR` 编写

**`edit` 参数**：
- `<name-or-id>` - 支持按名称或 ID 定位，优先按名称匹配

**例子**：
```bash
mkdir ~/config && cd ~/config
pkv note LyraX                 # 同步所有 note 到 ~/config/
pkv note LyraX list            # 列出
pkv note LyraX add --name "nginx.conf" --file /etc/nginx/nginx.conf  # 创建
pkv note LyraX edit "nginx.conf"  # 编辑
pkv note LyraX remove abc123def   # 删除
pkv note LyraX clean           # 清理
```

## 编辑器配置

`pkv note add` 和 `pkv env add` 命令在不使用 `--file` 选项时，以及 `edit` 命令会打开编辑器。

编辑器选择优先级：
1. 环境变量 `$EDITOR` 中指定的编辑器
2. Linux/macOS 默认降级到 `vi`
3. Windows 默认降级到 `notepad`

**设置编辑器**：
```bash
# 使用 vim
export EDITOR=vim

# 使用 VS Code（需安装 `code` 命令）
export EDITOR="code --wait"

# 使用 nano
export EDITOR=nano
```

编辑完成后保存退出即可（`:wq` 在 vim 中，`Ctrl+S` 后 `Ctrl+X` 在 nano 中）。

### 更新

```bash
pkv update                      # 检查并安装最新版本
```

### 版本信息

```bash
pkv --version                   # 显示版本、提交哈希、编译时间
```

## 工作原理

### SSH 密钥部署流程

1. 认证 Bitwarden（主密码 + 二次验证）
2. 同步个人密码库
3. 查找指定文件夹中所有 SSH Key 类型的 Item
4. 对每个 Key：
   - 提取私钥，写入 `~/.ssh/pkv_{keyname}` (权限 0600)
   - 提取公钥，写入 `~/.ssh/pkv_{keyname}.pub` (权限 0644)
   - 从 Notes 读取目标主机列表
   - 在 `~/.ssh/config` 中添加 Host 条目（使用 `>>> PKV MANAGED <<<` 标记块隔离）
5. 自动运行 `ssh-keyscan` 扫描所有目标主机，添加到 `~/.ssh/known_hosts`
6. 记录部署状态到 `~/.pkv/state.json`

### 清理流程

1. 读取 `~/.pkv/state.json` 中的部署记录
2. 删除所有 `~/.ssh/pkv_*` 密钥文件
3. 从 `~/.ssh/config` 中移除 PKV 标记块（保留其他手动配置）
4. 从 `~/.ssh/known_hosts` 中移除 PKV 标记块
5. 清空状态文件

**安全设计**：
- 使用标记注释隔离 PKV 管理的配置，清理时不会破坏你的手动配置
- 状态文件（`~/.pkv/state.json`）权限为 0600，只记录路径和元数据，不存储敏感数据
- 密钥文件权限为 0600，配置文件权限为 0600

## 目录结构

```
~/.ssh/
├── config                      # SSH 客户端配置
├── known_hosts                 # 已知主机指纹
├── pkv_*                       # PKV 管理的私钥
└── pkv_*.pub                   # PKV 管理的公钥

~/.pkv/
└── state.json                  # PKV 部署状态追踪
```

## 依赖

- **Bitwarden CLI**（`bw`） - 需要预先安装
  ```bash
  # macOS
  brew install bitwarden-cli
  
  # Linux
  sudo snap install bw
  # 或
  npm install -g @bitwarden/cli
  ```
  ```powershell
  # Windows
  winget install Bitwarden.CLI
  # 或
  choco install bitwarden-cli
  # 或
  scoop install bitwarden-cli
  ```
  > 如果未安装 `bw`，pkv 运行时会自动检测并给出当前平台的安装指引。
- Go 1.21+ （仅用于从源码构建）

## 安全考虑

- 🔒 PKV 不存储任何密钥或密码，所有敏感数据仅在运行时从 Bitwarden 获取
- 🔐 私钥文件权限自动设为 0600（仅所有者可读），配置文件权限为 0600
- 🛡️ 状态文件（`~/.pkv/state.json`）不包含任何机密，仅记录路径和时间戳
- 🔑 Bitwarden 主密码仅传递给 `bw` CLI，PKV 不触碰
- ✅ 所有代码已审计，无硬编码凭证

## 从源码构建

```bash
# 克隆仓库
git clone https://github.com/shichao402/pkv.git
cd pkv

# 构建
make build                      # 构建当前平台
make install                    # 构建并安装到 ~/.local/bin
make release                    # 交叉编译所有平台到 dist/
```

## 更新

使用 PKV 内置的自更新功能：
```bash
pkv update
```

或重新运行安装脚本：
```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/shichao402/pkv/main/install.sh | bash
```
```powershell
# Windows
irm https://raw.githubusercontent.com/shichao402/pkv/main/install.ps1 | iex
```

## 故障排除

### 问题：`bw: command not found`
**解决**：安装 Bitwarden CLI
```bash
brew install bitwarden-cli  # macOS
npm install -g @bitwarden/cli  # 通用
```

### 问题：SSH 连接时要求输入密码
**解决**：
1. 检查 SSH 密钥权限：`ls -la ~/.ssh/pkv_*` 应显示权限为 `-rw-------`
2. 尝试手动连接测试：`ssh -i ~/.ssh/pkv_keyname user@host`
3. 检查 `~/.ssh/config` 是否正确生成

### 问题：`known_hosts` 中没有主机记录
**解决**：
1. 确保 `ssh-keyscan` 已安装（通常与 openssh-clients 一起）
2. 手动运行：`ssh-keyscan -T 5 github.com >> ~/.ssh/known_hosts`

### 问题：更新失败
**解决**：
1. 检查网络连接
2. 确保当前版本小于最新版本
3. 检查 GitHub Releases 是否有对应平台的二进制文件

## 许可证

MIT

## 贡献

欢迎提交 Issue 和 Pull Request！

## 相关链接

- [Bitwarden 官网](https://bitwarden.com)
- [Bitwarden CLI 文档](https://bitwarden.com/help/cli/)
- [GitHub](https://github.com/shichao402/pkv)
