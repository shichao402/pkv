# pkv key add - SSH 密钥存储到 Bitwarden 设计文档

> 此文档从 Dec 项目迁移而来，原提交: 2541c39 (feat: add `dec key add`)
> 需要在 pkv 项目中重新实现

## 功能概述

添加 `pkv key add` 子命令，将 SSH 私钥导入 Bitwarden vault，存储为原生 SSH Key item (type 5)。
支持 PEM (PKCS1/PKCS8/EC) 和 OpenSSH 格式，使用纯 Go 自动转换，无需依赖外部 ssh-keygen。

## 依赖

- `golang.org/x/crypto/ssh` — 纯 Go SSH 密钥解析/转换/指纹生成
- `bw` CLI — Bitwarden 操作

## CLI 接口

```
pkv key add --priv ~/.ssh/id_rsa --pub "ssh-rsa AAAA..." --name "my-server-key"
pkv key add  # 交互式输入
```

### Flags

| Flag     | 说明                          | 必填 |
|----------|-------------------------------|------|
| `--priv` | 私钥文件路径                  | 否（交互式输入） |
| `--pub`  | 公钥内容 (ssh-rsa AAAA... 格式) | 否（交互式输入） |
| `--name` | 密钥名称                      | 否（交互式输入） |

## 核心流程

1. 获取私钥文件路径（flag 或交互输入）
2. 读取并解析私钥（支持 PEM PKCS1/PKCS8/EC/OpenSSH 格式）
3. 如非 OpenSSH 格式，自动转换为 OpenSSH 格式
4. 获取公钥（flag 或交互输入）
5. 生成 SHA256 指纹
6. 获取密钥名称（flag 或交互输入）
7. 确认信息后存储到 Bitwarden

## 核心函数设计

### parsePrivateKey(keyBytes []byte) (interface{}, ssh.Signer, error)

解析各种格式的私钥，返回原始密钥和 ssh.Signer。

解析顺序：
1. 先尝试 `ssh.ParsePrivateKey`（支持 OpenSSH 格式）
2. 回退到 PEM 解码 + `x509.ParsePKCS1PrivateKey` / `x509.ParsePKCS8PrivateKey` / `x509.ParseECPrivateKey`

```go
func parsePrivateKey(keyBytes []byte) (interface{}, ssh.Signer, error) {
    // 先尝试 ssh 库解析（支持 OpenSSH 格式）
    signer, err := ssh.ParsePrivateKey(keyBytes)
    if err == nil {
        rawKey, _ := ssh.ParseRawPrivateKey(keyBytes)
        return rawKey, signer, nil
    }

    // PEM 格式
    block, _ := pem.Decode(keyBytes)
    if block == nil {
        return nil, nil, fmt.Errorf("无法解码 PEM 数据")
    }

    var rawKey interface{}
    switch block.Type {
    case "RSA PRIVATE KEY":
        rawKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
    case "PRIVATE KEY":
        rawKey, err = x509.ParsePKCS8PrivateKey(block.Bytes)
    case "EC PRIVATE KEY":
        rawKey, err = x509.ParseECPrivateKey(block.Bytes)
    default:
        return nil, nil, fmt.Errorf("不支持的私钥类型: %s", block.Type)
    }
    if err != nil {
        return nil, nil, fmt.Errorf("解析私钥失败: %w", err)
    }

    signer, err = ssh.NewSignerFromKey(rawKey)
    if err != nil {
        return nil, nil, fmt.Errorf("创建 SSH signer 失败: %w", err)
    }
    return rawKey, signer, nil
}
```

### marshalToOpenSSH(rawKey interface{}) (string, error)

将原始私钥转换为 OpenSSH 格式字符串：

```go
func marshalToOpenSSH(rawKey interface{}) (string, error) {
    pemBlock, err := ssh.MarshalPrivateKey(rawKey, "")
    if err != nil {
        return "", fmt.Errorf("序列化为 OpenSSH 格式失败: %w", err)
    }
    return string(pem.EncodeToMemory(pemBlock)), nil
}
```

### generateFingerprint(signer ssh.Signer) (string, error)

生成 SHA256 指纹：

```go
func generateFingerprint(signer ssh.Signer) (string, error) {
    return ssh.FingerprintSHA256(signer.PublicKey()), nil
}
```

## Bitwarden 存储结构

```go
type bwItem struct {
    Type   int       `json:"type"`
    Name   string    `json:"name"`
    Notes  string    `json:"notes,omitempty"`
    SSHKey *bwSSHKey `json:"sshKey"`
}

type bwSSHKey struct {
    PrivateKey     string `json:"privateKey"`
    PublicKey      string `json:"publicKey"`
    KeyFingerprint string `json:"keyFingerprint"`
}
```

- Bitwarden SSH Key type = 5
- `bw create item` 需要 base64 编码的 JSON
- 创建前需确保 `bw` 已解锁（检查 BW_SESSION 环境变量或提示输入主密码）

### createBWSSHKey 流程

```go
func createBWSSHKey(session, name, privateKey, publicKey, fingerprint string) error {
    item := bwItem{
        Type: 5,
        Name: name,
        SSHKey: &bwSSHKey{
            PrivateKey:     strings.TrimSpace(privateKey),
            PublicKey:      strings.TrimSpace(publicKey),
            KeyFingerprint: fingerprint,
        },
    }
    jsonData, _ := json.Marshal(item)
    encoded := base64.StdEncoding.EncodeToString(jsonData)

    args := []string{"create", "item", encoded}
    if session != "" {
        args = append(args, "--session", session)
    }
    cmd := exec.Command("bw", args...)
    output, err := cmd.CombinedOutput()
    // ...
}
```

## ensureBWUnlocked 流程

1. 检查 `BW_SESSION` 环境变量 → 验证是否有效
2. 检查 `bw status` → 如 unauthenticated 则提示登录
3. 如 locked 则提示输入主密码 → `bw unlock <password> --raw`
4. 返回 session key

## 测试覆盖（21 个测试）

### parsePrivateKey 测试
- TestParsePrivateKey_RSAPKCS1 — RSA PKCS1 PEM 解析
- TestParsePrivateKey_RSAPKCS1_4096 — RSA 4096 位密钥
- TestParsePrivateKey_RSAPKCS8 — RSA PKCS8 PEM 解析
- TestParsePrivateKey_EC — EC P256 PEM 解析
- TestParsePrivateKey_OpenSSH — OpenSSH 格式解析
- TestParsePrivateKey_InvalidData — 无效数据错误处理
- TestParsePrivateKey_EmptyInput — 空输入错误处理
- TestParsePrivateKey_UnsupportedPEMType — 不支持的 PEM 类型
- TestParsePrivateKey_CorruptedPEM — 损坏的 PEM 数据

### marshalToOpenSSH 测试
- TestMarshalToOpenSSH_FromRSA — RSA 转 OpenSSH
- TestMarshalToOpenSSH_FromEC — EC 转 OpenSSH
- TestMarshalToOpenSSH_RoundTrip — PEM → OpenSSH 往返一致性
- TestMarshalToOpenSSH_RSA4096RoundTrip — RSA 4096 往返

### generateFingerprint 测试
- TestGenerateFingerprint_RSA — RSA 指纹生成
- TestGenerateFingerprint_EC — EC 指纹生成
- TestGenerateFingerprint_Deterministic — 指纹确定性
- TestGenerateFingerprint_DifferentKeysHaveDifferentFingerprints — 不同密钥指纹不同

### JSON 序列化测试
- TestBWItemJSON_Structure — JSON 结构完整性
- TestBWItemJSON_OmitsEmptyNotes — 空 notes 省略

### 其他
- TestTruncate — 字符串截断
- TestEndToEnd_PEMToOpenSSHWithFingerprint — 端到端完整流程

## 完整源码参考

原始实现文件见本文档同级提交历史，或参考上述函数设计直接在 pkv 项目中重新实现。
