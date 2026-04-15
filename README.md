# PKV - Personal Key Vault

**PKV** 是一个围绕 Bitwarden 组织的命令行工具，用来把一个 folder 里的三类资源落到本地：

- SSH Key Item：部署到 `~/.ssh/`
- 一个保留名为 `pkv.env` 的 Secure Note：生成 env 产物文件
- 其他 Secure Note：同步成当前项目目录里的配置文件

当前版本的核心目标只有两个：

- 命令结构直观，先选动作，再选 folder，再选资源类型
- 明确本地和远端的对齐规则，避免“到底谁覆盖谁”不清楚

## 命令模型

新的命令只有这一套：

```bash
pkv list [folder]
pkv get <folder> <ssh|env|note>
pkv add <folder> <ssh|env|note>
pkv edit <folder> <env|note> [name-or-id]
pkv remove <folder> <ssh|env|note> [id...]
pkv clean <folder> <ssh|env|note>
pkv update
```

旧命令模型已经移除，不再维护兼容层。

如果直接执行 `pkv`，会进入交互模式：

```text
$ pkv
Interactive mode. Type 'help' for commands, 'exit' to quit.
Examples: 'get dev env' or 'dev env'.
pkv>
```

交互模式里同一个 `pkv` 进程会把 `BW_SESSION` 保持在内存中，所以你在一次会话里连续执行多条命令，不需要每次都重新输入主密码。

现在也支持常见终端操作：

- `↑` / `↓` 切换历史命令
- `Ctrl+R` 反向搜索历史命令

## 安装

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/shichao402/pkv/main/install.sh | bash
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/shichao402/pkv/main/install.ps1 | iex
```

验证：

```bash
pkv --version
```

## Bitwarden 数据组织

PKV 的组织单位是 **folder**。一个 folder 通常对应一个环境、一个项目，或者一个“可以一起落地”的秘密集合。

### 1. SSH

使用 Bitwarden 原生的 **SSH Key** Item。

要求：

- 类型：`SSH Key`
- 名称：任意，例如 `github-prod`
- `Notes`：写目标主机，一行一个

示例：

```text
github.com
10.0.0.12
10.0.0.13:2222
*.corp.internal
```

`pkv get <folder> ssh` 会把这些 Key 部署到本地，并基于 `Notes` 生成 `~/.ssh/config` 和 `known_hosts` 的 PKV 管理区块。

### 2. Env

一个 folder 里只允许一个 env item，使用 **Secure Note**，名字固定为：

```text
pkv.env
```

要求：

- 类型：`Secure Note`
- 名称：`pkv.env`
- 内容：`KEY=VALUE`，一行一个

示例：

```text
DB_HOST=127.0.0.1
DB_USER=app
DB_PASS="secret"
REDIS_URL=redis://127.0.0.1:6379/0
```

说明：

- 新版本不再要求 `pkv_type=env` 字段。
- 历史上已经用 `pkv_type=env` 标过的旧数据仍然能识别，便于迁移。
- `pkv get <folder> env` 不会修改系统环境变量，它只生成本地产物文件。

### 3. Note

同一个 folder 里，除了 `pkv.env` 之外的其他 **Secure Note**，都视为“配置文件模板”。

要求：

- 类型：`Secure Note`
- 名称：目标文件名，例如 `app.secrets.json`、`.env.local`、`config.yaml`
- 内容：文件正文

可选元数据字段：

- `pkv_note_strategy`：note 落盘策略，默认 `file`
- `pkv_note_target`：显式覆盖本地目标路径

当前支持的策略：

- `file`：默认模式，沿用 note 名称或 `pkv_note_target` 作为落盘路径
- `mise_conf_d`：要求目标落在 `.config/mise/conf.d/*.toml`，适合 mise 这类支持碎片目录的工具

示例：

- Note 名称：`app.secrets.json`
- Note 内容：一整份 JSON

`pkv get <folder> note` 会把这些 note 同步到当前目录，文件名直接使用 note 名称。
如果 note 名称里包含路径，例如 `lyra/test/note`，就会按这个目录结构写到当前目录下。
出于安全考虑，不允许使用绝对路径或 `..` 逃逸出当前目录。

如果 note 设置了 `pkv_note_strategy=mise_conf_d`：

- 默认会落到 `.config/mise/conf.d/pkv-<folder>-<note>.toml`
- 也可以用 `pkv_note_target` 显式指定 `.config/mise/conf.d/*.toml`
- 这样不同来源可以各写各的 fragment，避免都去争抢单个 `mise.toml` / `mise.local.toml`

## 快速上手

### 1. 看有哪些 folder

```bash
pkv list
```

### 2. 看某个 folder 里有什么

```bash
pkv list prod
```

输出会按资源分组，告诉你：

- 有多少 SSH Key
- 有没有 `pkv.env`
- 有多少普通配置 note

### 3. 拉取 SSH

```bash
pkv get prod ssh
```

这会：

- 从 Bitwarden 同步 `prod` folder 里的所有 SSH Key
- 写入 `~/.ssh/pkv_*`
- 更新 `~/.ssh/config`
- 更新 `~/.ssh/known_hosts`
- 把部署状态写进 `~/.pkv/state.json`

### 4. 生成 env 产物

```bash
pkv get prod env
```

这会生成三份文件：

```text
~/.pkv/env/prod.json
~/.pkv/env/prod.sh
~/.pkv/env/prod.ps1
```

推荐使用方式：

- shell 场景：`source ~/.pkv/env/prod.sh`
- 应用程序场景：直接读取 `~/.pkv/env/prod.json`
- 更合理的长期做法：应用程序直接读取自己约定好的配置文件，而不是依赖全局环境注入

### 5. 同步项目配置文件

先进入项目目录：

```bash
cd ~/workspace/my-app
pkv get prod note
```

这会把 `prod` folder 里的普通 Secure Note 同步到当前目录。

例如：

- `app.secrets.json` -> `~/workspace/my-app/app.secrets.json`
- `.env.local` -> `~/workspace/my-app/.env.local`

## 交互模式

直接运行：

```bash
pkv
```

交互模式里既支持完整命令，也支持简写。

### 完整命令

```text
pkv> list
pkv> list prod
pkv> get prod ssh
pkv> get prod env
pkv> get prod note
```

### 简写命令

```text
pkv> prod list
pkv> prod ssh
pkv> prod env
pkv> prod note
pkv> prod env clean
pkv> prod note add --name app.secrets.json --file ./app.secrets.json
```

退出方式：

```text
pkv> exit
```

## 常用命令

### `list`

```bash
pkv list
pkv list <folder>
```

用途：

- `pkv list`：列出 Bitwarden 里的 folder
- `pkv list <folder>`：列出这个 folder 下的 SSH、env、note 概况

### `get`

```bash
pkv get <folder> ssh
pkv get <folder> env
pkv get <folder> note
```

用途：

- `ssh`：把远端 SSH Key 落到本地
- `env`：把 `pkv.env` 物化为本地产物文件
- `note`：把普通 Secure Note 同步到当前目录

### `add`

```bash
pkv add <folder> ssh --priv ~/.ssh/id_ed25519 --name github-prod
pkv add <folder> env --file .env.prod
pkv add <folder> note --name app.secrets.json --file ./app.secrets.json
```

说明：

- `add ssh`：向 Bitwarden 新建 SSH Key Item
- `add env`：创建或覆盖这个 folder 的 `pkv.env`
- `add note`：创建一个普通配置 note
- `add env` / `add note` 如果不传 `--file`，会打开 `$EDITOR`

### `edit`

```bash
pkv edit <folder> env
pkv edit <folder> note <name-or-id>
```

说明：

- `edit env`：编辑 `pkv.env`
- `edit note`：按名称或 ID 编辑某个配置 note

### `remove`

```bash
pkv remove <folder> env
pkv remove <folder> ssh <id> [id2]...
pkv remove <folder> note <id> [id2]...
```

说明：

- `remove` 会删除 Bitwarden 里的远端资源
- 对于已经落地到本地的资源，PKV 会尽量顺手清理本地产物

### `clean`

```bash
pkv clean <folder> ssh
pkv clean <folder> env
pkv clean <folder> note
```

说明：

- `clean` 只清理本地，不删除 Bitwarden 里的数据
- `clean <folder> note` 只清理**当前目录**里这份同步结果

## 本地与远端如何对齐

这是 PKV 设计里最重要的一部分。

### SSH 的对齐规则

远端是唯一事实来源。

执行 `pkv get <folder> ssh` 时：

- 远端新增 Key：本地新增部署
- 远端删除 Key：本地已追踪的旧 key 文件、config 条目会被移除
- 远端重命名 Key：本地会按新名字重新部署，并更新相关配置
- 本地手动改 `~/.ssh/pkv_*`：不建议，下一次 `get` 可能被重写

状态追踪内容大致是：

- Bitwarden item ID
- 本地 key 文件路径
- 主机列表
- 当前 folder

### Env 的对齐规则

远端也是唯一事实来源。

执行 `pkv get <folder> env` 时：

- 远端存在 `pkv.env`：重新生成 `json/sh/ps1` 三份文件
- 远端删除 `pkv.env`：本地已追踪的 env 产物会在下一次 `get` 时被清理
- 本地手改这些产物：不建议，下一次 `get` 会重写

状态追踪内容大致是：

- env item ID
- folder
- 产物路径
- 包含了哪些 key

要点：

- PKV 不再做“持久写入系统环境变量”的事情
- env 在 PKV 里是“从远端生成本地文件”，不是“替你接管机器环境”

### Note 的对齐规则

Note 和 SSH / env 不一样，因为它直接落在项目目录，最容易和本地手工修改冲突。

PKV 当前的规则是：

- 追踪维度是 `folder + targetDir + itemID`
- 同一个 folder 可以同步到多个不同目录，各自独立追踪
- 远端新增 note：本地创建新文件
- 远端重命名 note：本地已追踪文件会跟着改名
- 远端修改内容：本地已追踪文件会更新
- 远端删除 note：本地已追踪文件会删除

但有一个保护规则：

- 如果本地已追踪文件在上次同步后被你手工改过，PKV 会拒绝覆盖
- 如果远端 note 已经删了，但本地文件被你手工改过，PKV 也会拒绝删除

这意味着：

- 你要改远端内容，应该用 `pkv edit <folder> note <name-or-id>`
- 你要接受远端版本，先删除本地冲突文件，或者 `pkv clean <folder> note` 后再 `pkv get <folder> note`
- 如果当前目录已经有一个**未被 PKV 追踪**的同名文件，PKV 也不会直接覆盖它

如果某个 note 使用碎片化策略，例如 `pkv_note_strategy=mise_conf_d`：

- PKV 仍然按同样的追踪和冲突规则工作
- 但实际落点会变成 `.config/mise/conf.d/*.toml` 这类 fragment 文件
- 这样多个来源可以各自拥有稳定片段，减少对单个原子配置文件的争抢

状态追踪内容大致是：

- Bitwarden item ID
- folder
- 目标目录
- 文件路径
- 上次同步内容的 hash

## 推荐的数据组织方式

如果你希望长期用得顺手，建议按下面的方法组织 Bitwarden：

- 一个项目一个 folder，例如 `my-app-dev`、`my-app-prod`
- 一个 folder 里最多一个 `pkv.env`
- 每个真正需要落地成文件的机密配置，各自建一个 Secure Note
- SSH 主机说明写在 SSH Key 的 `Notes` 里，而不是再额外建 note

一个典型 folder 可能长这样：

```text
Folder: my-app-prod

- SSH Key: deploy
- SSH Key: github-actions
- Secure Note: pkv.env
- Secure Note: app.secrets.json
- Secure Note: .env.runtime
- Secure Note: redis.conf
```

这样做的好处是：

- `pkv list my-app-prod` 一眼就能看懂
- `pkv get my-app-prod env` 和 `pkv get my-app-prod note` 的行为边界很清楚
- 不需要再靠额外 tag 去猜 note 到底是什么用途

## 编辑器

以下命令在不传 `--file` 时，会打开编辑器：

- `pkv add <folder> env`
- `pkv add <folder> note --name <name>`
- `pkv edit <folder> env`
- `pkv edit <folder> note <name-or-id>`

编辑器优先级：

1. `$EDITOR`
2. macOS / Linux 下默认 `vi`
3. Windows 下默认 `notepad`

示例：

```bash
export EDITOR="code --wait"
```

## 本地产物与状态文件

PKV 会写这些位置：

```text
~/.ssh/config
~/.ssh/known_hosts
~/.ssh/pkv_*
~/.pkv/state.json
~/.pkv/env/<folder>.json
~/.pkv/env/<folder>.sh
~/.pkv/env/<folder>.ps1
<current-dir>/<note-name>
```

`~/.pkv/state.json` 不保存私钥、密码、note 正文，只保存对齐所需的追踪信息。

## 依赖

- Bitwarden CLI：`bw`
- Go 1.21+（仅源码构建时需要）

安装 Bitwarden CLI 示例：

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

## 从源码构建

```bash
git clone https://github.com/shichao402/pkv.git
cd pkv
make build
make install
make release
```

## 故障排查

### `bw: command not found`

先安装 Bitwarden CLI，并确保 `bw` 在 PATH 中。

### 交互模式里 `BW_SESSION` 明明导出了，还是提示输入主密码

先确认导出的 session 还有效：

```bash
bw --nointeraction --session "$BW_SESSION" list folders
```

如果输出类似 `Vault is locked.`，说明这段 session 已经失效，重新执行：

```bash
export BW_SESSION="$(bw unlock --raw)"
```

然后再进交互模式：

```bash
PKV_DEBUG=1 pkv
pkv> dev env
```

### `pkv get <folder> note` 报文件冲突

PKV 现在会先做一次完整预检：

- 会一次性列出本轮所有已发现的冲突
- 只要预检失败，本轮不会删除、改名或写入任何本地 note 文件

常见情况包括：

- 当前目录已经存在未追踪的同名文件
- 已追踪文件被你手工改过，PKV 拒绝覆盖
- 两个远端 note 解析到同一个本地路径，或者一个要求文件、另一个要求目录

处理方式：

- 先确认本地文件是否要保留
- 不保留就删除后重新 `pkv get <folder> note`
- 或者先 `pkv clean <folder> note` 再重新同步

### SSH 已部署但连接不对

检查：

- `pkv list <folder>` 看远端是否真的有目标 key
- SSH Key 的 `Notes` 是否正确填写了 host / host:port
- `~/.ssh/config` 里是否生成了 PKV 管理区块

### 打开排查日志

设置 `PKV_DEBUG=1` 后，PKV 会输出脱敏诊断日志，例如会话是否被复用、执行了哪类 Bitwarden 命令、env 产物写到了哪些路径，但不会打印 `BW_SESSION`、私钥或 env value 原文。

## 更新

```bash
pkv update
```

## 许可证

MIT
