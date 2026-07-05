# PKV - Personal Key Vault

> **项目状态：不再推荐作为首选方案**
>
> PKV 仍可安装和使用，但**已不是管理 Bitwarden 秘密与项目配置的最优方式**。
>
> 请改用 **[Dec](https://github.com/shichao402/Dec)** —— 个人 AI 知识仓库，已**原生集成 Bitwarden**，在拉取 Skills / Rules / MCP 资产的同时自动同步 secrets bundle，一条 TUI 流程即可完成，比单独维护 PKV 更方便。

## 为什么改用 Dec

| 维度 | PKV | Dec |
|------|-----|-----|
| 定位 | 独立的 Bitwarden → 本地落地 CLI/TUI | AI 资产仓库 + Bitwarden secrets bundle **同构绑定** |
| 组织方式 | 按 Bitwarden folder 手动映射项目 | Project → Bundle，Dec Git 与 Bitwarden **同名 bundle 成对拉取** |
| 配置文件 | Secure Note 名即文件名，需自行约定 | Note 名 = **项目根相对路径**（如 `.config/mise/conf.d/app.toml`） |
| SSH | `~/.ssh/pkv_*` + PKV 管理 config 区块 | `~/.ssh/dec_<bundle>_<name>` + Dec 管理 config 区块 |
| IDE 集成 | 需单独部署 `dec-pkv-mcp` | TUI **Run** 页一次 pull：Dec 资产 + secrets 同时落地 |
| 交互入口 | `pkv` TUI / CLI | `dec` TUI（Settings / Home / Assets / Run） |

Dec 把公开资产（Skills、Rules、MCP）和私密文件（env、mise 配置、SSH Key）放在同一套 **bundle** 模型里管理；`dec pull` 会先拉 Git bundle 到 `.dec/cache/`，再自动拉 Bitwarden secrets bundle 到项目根或 `~/.ssh/`，无需再单独跑 `pkv get`。

详细模型见 Dec 文档：[BUNDLE-SECRETS-MODEL.md](https://github.com/shichao402/Dec/blob/main/Documents/BUNDLE-SECRETS-MODEL.md)

## 快速迁移

1. **安装 Dec**

   ```bash
   curl -fsSL https://raw.githubusercontent.com/shichao402/Dec/ReleaseLatest/scripts/install.sh | bash
   ```

2. **在项目目录启动 TUI**

   ```bash
   dec
   ```

3. **Settings** 页连接资产仓库并配置 Bitwarden；**Home** 页初始化 project；**Assets** 页选择 bundle；**Run** 页拉取。

4. 将原 PKV folder 中的 Secure Note 按 **项目相对路径** 重命名并迁入对应 secrets bundle；SSH Key 迁入同名 bundle 后由 Dec 部署到 `~/.ssh/`。

5. 新项目直接使用 `.dec/config.yaml` + Dec project，不再依赖 `.pkv/workspace.yaml` 或 `pkv_folder`。

## PKV 仍可用（遗留）

若你已在用 PKV、暂未完成迁移，下列能力仍受维护，但**新用户请直接上 Dec**。

### 安装

```bash
curl -fsSL https://raw.githubusercontent.com/shichao402/pkv/ReleaseLatest/install.sh | bash
pkv --version
```

Windows (PowerShell)：

```powershell
irm https://raw.githubusercontent.com/shichao402/pkv/ReleaseLatest/install.ps1 | iex
```

### 命令概览

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

交互式终端默认进入 TUI（`pkv`）；脚本 / CI 使用 `PKV_NO_TUI=1 pkv ...`。

### Bitwarden 数据约定（遗留）

- **SSH**：Bitwarden SSH Key Item，`Notes` 写目标主机
- **Env**：folder 内唯一 Secure Note，名称固定 `pkv.env`，内容 `KEY=VALUE`
- **Note**：其余 Secure Note 按名称同步到当前目录

### 依赖

- Bitwarden CLI：`bw`（`brew install bitwarden-cli` 等）
- Go 1.21+（仅源码构建）

### 从源码构建

```bash
git clone https://github.com/shichao402/pkv.git
cd pkv
make build && make install
```

## 相关链接

- **推荐替代**：[Dec](https://github.com/shichao402/Dec)
- PKV 仓库：<https://github.com/shichao402/pkv>
- Dec secrets 模型：[Documents/BUNDLE-SECRETS-MODEL.md](https://github.com/shichao402/Dec/blob/main/Documents/BUNDLE-SECRETS-MODEL.md)

## 许可证

MIT
