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
pkv get <folder> <ssh|env|note|all>
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

对于连续的只读命令，PKV 还会在同一进程里短时复用最近一次成功的 `bw sync` 结果，避免每条命令都重复触发一次远端同步。
一旦本轮进程里成功执行了新增、编辑、删除等 Bitwarden 写操作，下一次读取仍会重新 `bw sync`，不会继续复用旧结果。

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

示例：

- Note 名称：`app.secrets.json`
- Note 内容：一整份 JSON

`pkv get <folder> note` 会把这些 note 同步到当前目录，文件名直接使用 note 名称。
如果 note 名称里包含路径，例如 `lyra/test/note`，就会按这个目录结构写到当前目录下。
出于安全考虑，不允许使用绝对路径或 `..` 逃逸出当前目录。

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

### 6. 在新服务器上就地生成 SSH Key

新开一台机器、想直接用 SSH key 免密登录，但又不想在本地生成完再 `scp` 上去？在服务器上手动登录后执行：

```bash
# 服务器上：生成 keypair，私钥直传 Bitwarden
pkv add prod ssh --generate --name web01-prod --host web01.example.com

# 服务器上：把公钥装进自己的 authorized_keys
pkv get prod ssh --authorize
```

发生了什么：

- `add --generate` 在内存里生成一对 ed25519 keypair（默认；`--type rsa --bits 4096` 可选）
- 私钥以 OpenSSH 格式写进 Bitwarden 的 SSH Key Item，**完全不落本地磁盘**
- `--host` 写入条目的 `Notes`（一行一个），客户端 pull 后自动生成 ssh-config 别名；不传 `--host` 时回退到本机 hostname
- `get --authorize` 把 folder 里每把 key 的公钥追加到当前机器的 `~/.ssh/authorized_keys`，去重，自动修正 `~/.ssh` 700 / `authorized_keys` 600

回到客户端：

```bash
pkv get prod ssh
ssh web01.example.com   # 免密成功
```

常用 flag：

- `add` 侧
  - `--type ed25519|rsa`：算法，默认 `ed25519`
  - `--bits N`：仅 RSA 使用，必须 ≥ 2048，默认 4096
  - `--comment STR`：公钥注释，默认 `<user>@<hostname> (pkv)`
  - `--host H`：可重复（`--host a --host b`）或逗号分隔（`--host a,b`）
- `get` 侧
  - `--authorize`：把每把公钥追加到本机 `~/.ssh/authorized_keys`，用于在目标机器上完成 ssh-copy-id 的角色

注意：

- `--generate` 与 `--priv` / `--pub` 互斥
- `--authorize` 失败不会回滚部署（部署成功 vs authorize 是两件事）；失败时打 warning，可手动 `cat ~/.ssh/pkv_<name>.pub >> ~/.ssh/authorized_keys`
- 私钥仅经过 BW，本地任何时候都没有副本，丢失主密码即丢失这把 key

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
pkv get <folder> all
```

用途：

- `ssh`：把远端 SSH Key 落到本地
- `env`：把 `pkv.env` 物化为本地产物文件
- `note`：把普通 Secure Note 同步到当前目录
- `all`：一次性执行 ssh + env + note 三个子命令，失败不中断，末尾聚合报错

### `add`

```bash
pkv add <folder> ssh --priv ~/.ssh/id_ed25519 --name github-prod
pkv add <folder> ssh --generate --name web01-prod --host web01.example.com
pkv add <folder> env --file .env.prod
pkv add <folder> note --name app.secrets.json --file ./app.secrets.json
```

说明：

- `add ssh`：向 Bitwarden 新建 SSH Key Item
  - `--priv` 模式：从本地已有私钥导入
  - `--generate` 模式：在内存里现场生成 keypair，私钥直接写进 Bitwarden（**不落本地**）；如果在目标服务器上跑、想顺手把公钥装进自己的 `authorized_keys`，紧接着用 `pkv get <folder> ssh --authorize`；典型用法见下文「在新服务器上就地生成 SSH Key」
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

### `unlock`

```bash
pkv unlock
pkv unlock --export
```

用途：

- 解锁 Bitwarden 并把 session 打到 stdout，便于 `$(...)` / `eval` 复用
- 已有有效 `BW_SESSION` 时直接复用，不再询问主密码
- 详细用法见下文「自动化与脚本化（BW_SESSION）」一节

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

## 自动化与脚本化（BW_SESSION）

PKV 默认会在需要时打开 TTY 提示主密码。如果你要在脚本、CI 或者多窗口场景里复用解锁状态，**正路是复用 Bitwarden session**，而不是把主密码塞进环境变量或命令行。

PKV 启动时会优先读环境变量 `BW_SESSION`，并用 `bw list folders` 探一次看 session 是否还活着。活着就直接复用，不会再提示。

### `pkv unlock`

PKV 提供了一个专门解锁并输出 session 的子命令，避免你记 `bw` 的具体用法：

```bash
# 默认把 session 字符串打到 stdout，方便 $(...) 捕获
export BW_SESSION="$(pkv unlock)"

# 或者直接输出可以 eval 的 shell 语句
eval "$(pkv unlock --export)"
```

特性：

- **stdout 只放 session**，所有提示（"Authenticating..."）都打到 stderr，不会污染 `$(...)` 结果
- 如果 `BW_SESSION` 已经存在且有效，**直接复用并打印**，不会重新提示主密码
- 如果 session 已失效或未设置，走 `bw unlock --raw` 交互式询问主密码
- PKV **从不**接受主密码参数，全部交互由 `bw` 在 TTY 上处理

### 一次解锁，多次使用

```bash
export BW_SESSION="$(pkv unlock)"
pkv get lyra note
pkv get vikunja-tasker env
pkv list
```

同一个 shell 窗口里，所有 PKV 命令都不再提示主密码，直到 session 到期（默认 Bitwarden vault timeout）或你 `bw lock` 主动失效它。

### 放进你的 shell 里

如果想避免重复敲 `pkv unlock`，可以加一个显式的 alias，按需触发：

```bash
# ~/.zshrc / ~/.bashrc
alias pkv-unlock='eval "$(pkv unlock --export)"'
```

然后：

```bash
pkv-unlock            # 每天进终端时手动触发一次
pkv get lyra note
pkv get tencent-cloud ssh
```

注意：**不要在 shell 启动时自动执行 `pkv unlock`**，那会把"解锁"从显式动作变成隐式动作，和保存主密码到磁盘没有本质区别。

### CI / 无人值守场景

CI 里的典型做法是：主密码只作为**外部 secret** 注入到 `bw unlock` 的环境，解锁后立刻释放。这一步绕过 `pkv unlock`，直接用 `bw`：

```bash
# CI secret 管理器已经把 BW_PASSWORD 注进这一步
export BW_SESSION="$(bw unlock --raw --passwordenv BW_PASSWORD)"
unset BW_PASSWORD

pkv get lyra note
pkv get vikunja-tasker env
```

这样 `pkv` 本身从头到尾不接触主密码，`BW_SESSION` 只在本次 job 生命周期内有效——即使泄漏也能通过 `bw lock` 单独失效这一条会话，不用轮换整个 vault。

### 交互模式里的 unlock

在 `pkv>` REPL 里也可以手动触发一次解锁来预热 session：

```text
pkv> unlock
Authenticating with Bitwarden...
Vault unlocked.
pkv> lyra note
```

REPL 里 `unlock` **不会把 session 打到屏幕上**，只确认解锁成功。session 会留在当前进程内，后续命令复用。

### 为什么 PKV 不提供 `--master-pass`

这是一次刻意的取舍：

- **Blast radius**：`BW_SESSION` 泄漏 = 这条 session 过期前可读；主密码泄漏 = 整个 vault 永久失守，必须改密码 + 轮换所有秘密
- **不变量**：PKV 应当永远不接触主密码。这条约束在代码层强于任何文档警告
- **功能没少**：`BW_SESSION` 已经覆盖所有自动化场景，包括一次解锁跨多命令复用

如果你的使用场景看起来缺了 `--master-pass` 才能做到，多半说明正确做法是先 `pkv unlock`（或 `bw unlock --raw`）拿 session，再让 PKV 使用它。

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

PKV 启动时会先检查 `bw` 是否在 PATH 中，并额外探测一次 `bw --version`。
如果这个命令失败、没有输出，或者输出不是正常版本号，PKV 会在进入 Bitwarden 认证前直接报错。

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

### 启动时提示 `bw --version` 异常或版本不可识别

PKV 现在会在认证前先探测 `bw --version`。

先单独运行：

```bash
bw --version
```

预期输出应该是普通版本号，例如：

```text
2026.2.0
```

如果这个命令失败、没有输出，或者被 alias / wrapper 改写成其他文本，先修复 `bw` 安装或调用方式，再重新执行 PKV。

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
