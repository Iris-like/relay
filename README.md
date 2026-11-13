# Relay：自更新

目标：亲手实现「检查 → 下载 → 验签 → 替换 → 重启」，并验证篡改会被拒绝。

不依赖 Tauri、不依赖 GUI，用一个 Go 小程序把机制跑通。

---

## 目录

```
.
├── main.go                  version / check / update / guard / block 命令
├── feed.go                  feed 结构与版本比较
├── verify.go                Ed25519 验签 + 解析签名清单
├── policy.go                策略签名、封禁列表、本机状态
├── install.go               下载、哈希校验、替换、exec
├── scripts/
│   ├── gen-keys.sh          生成签名密钥
│   ├── build-client.sh      编译带版本号与公钥的客户端
│   ├── make-feed.sh         编译新版本并生成 feed
│   └── gen-feed.py          生成 SHA256SUMS、签名与 feed.json
├── .github/workflows/release.yml  tag 触发构建、签名、发布
└── server/                  本地 feed 与产物（gitignore）
```

---

## 快速开始

### 1. 生成密钥

```bash
bash scripts/gen-keys.sh
```

### 2. 编译「旧版本」并安装到本地

```bash
bash scripts/build-client.sh 0.1.6 install/relay
./install/relay version
# relay 0.1.6 darwin/arm64
```

公钥会被编译进客户端，私钥只在本地 `.keys/`。

### 3. 生成「新版本」与 feed

```bash
bash scripts/make-feed.sh 0.1.7
```

产物：

```
server/0.1.7/relay-darwin-arm64
server/0.1.7/relay-darwin-amd64
server/0.1.7/relay-linux-amd64
server/0.1.7/SHA256SUMS
server/feed.json
```

### 4. 起本地 feed 服务

```bash
python3 -m http.server 8099 --bind 127.0.0.1 --directory server
```

### 5. 检查并更新

另开一个终端：

```bash
./install/relay check
# latest 0.1.7, current 0.1.6
# update available

./install/relay update
# 发现新版本 0.1.7，下载 relay-darwin-arm64
# 签名与哈希校验通过，准备替换
# relay 0.1.7 darwin/arm64

./install/relay version
# relay 0.1.7 darwin/arm64
```

---

## 接上 GitHub Release

现在采用和 magpie 相同的双仓库结构：

```
relay             源码 + 手动调试构建（workflow_dispatch）
relay-releases    构建、签名、发布、feed（Anvil 流水线）
```

`relay-releases` 的 workflow 会 checkout `relay` 的源码，构建 `relay-*`，签名后发布到自己的 Releases；`relay` 仓库不再由 tag 自动发布。

更新命令：

```bash
./install/relay update \
  --feed https://github.com/<owner>/relay-releases/releases/latest/download/relay-feed.json
```

实测输出（0.2.0 → 0.3.2）：

```
发现新版本 0.3.2，下载 relay-darwin-arm64
签名与哈希校验通过，准备替换
relay 0.3.2 darwin/arm64
```

三个前提：

1. **两个仓库都要公开**。客户端没有 GitHub token，私有仓库的资产下载不了。
2. feed 作为 Release 资产发布，名字用 `relay-feed.json`；实测叫 `feed.json` 时匿名下载会 404。
3. feed 里的资产 URL 指向 `relay-releases` 对应 tag 的下载地址，不是源码仓库。

这就是 Anvil 的发布流程与 Relay 的更新逻辑合在一起后的完整闭环。

### 自动镜像

`relay` 仓库带 `.github/workflows/mirror.yml`：

- 每小时 :17 拉取 `relay-releases` 的最新 Release
- 已存在同 tag 时比对正文和资产：正文变了就更新，资产名或大小变了就重传，都一样则不动
- 只在版本不旧于现有 Latest 时打 Latest，最后再按版本号校正一次
- **读源仓库用匿名 GitHub API**，不需要跨仓库 PAT
- 写回本仓库用自带的 `GITHUB_TOKEN`

手动触发：

```bash
gh workflow run mirror.yml -R <owner>/relay
```

实测：`relay-releases` 发布 v0.3.3 后，镜像 workflow 建出同名、同资产的 Release，两边 Latest 都是 v0.3.3。

注意：镜像不复制 tag，源码仓库的 tag 需要先存在（发布流程里先给 `relay` 打 tag，再给 `relay-releases` 打 tag）。

注意：从 `lab02` 改名到 `relay` 是不兼容变更——旧客户端验签的消息前缀、寻找的资产名都变了，无法跨名字更新。旧机器需要手动安装一次 `relay`，之后才能走自更新。

兼容性也验证过：v0.1.7 的旧客户端不认识 `min_version` / `rollback` / `blocked`，仍能正常升级到 v0.1.8；新客户端才开始执行这些策略。

---

## 更新时发生了什么

```
GET /feed.json
  ↓
semver 比较：0.1.7 > 0.1.6
  ↓
校验签名：ed25519("relay release 0.1.7\n" + SHA256SUMS)
  ↓
从签名清单里取当前平台产物的 sha256
  ↓
下载到 .relay.new（与可执行文件同目录）
  ↓
计算 sha256，必须等于清单里的值
  ↓
旧文件改名 relay.old，新文件改名 relay
  ↓
syscall.Exec 用新二进制替换当前进程
  ↓
新版本连续两次通过 guard 启动检查后清理 relay.old
```

两个关键点：

1. **先验签，再信哈希**。哈希来自被签名的清单，不是来自 feed 字段。
2. **暂存文件与目标同目录**，保证 rename 在同一个文件系统上，是原子操作。

---

## 策略：封禁、回滚、强制更新

feed 增加三个字段，全部由 `meta_sig` 单独签名：

| 字段 | 作用 |
| --- | --- |
| `blocked` | 列出不能安装的版本 |
| `rollback` | 允许 feed 指向比当前旧的版本 |
| `min_version` | 低于它拒绝运行 |

为什么单独签名：`SHA256SUMS` 只覆盖文件，不覆盖策略。如果只签文件，攻击者可以随意把 `min_version` 改成不可能满足的版本，或把 `rollback` 打开。

### 命令

```bash
./install/relay check
./install/relay update
./install/relay guard                 # 低于 min_version 时退出码 1
./install/relay block 0.1.9           # 本机封禁
./install/relay blocked
./install/relay unblock 0.1.9
```

### 生成带策略的 feed

```bash
BLOCKED=0.1.9       bash scripts/make-feed.sh 0.1.9
ROLLBACK=1          bash scripts/make-feed.sh 0.1.8
MIN_VERSION=0.2.0   bash scripts/make-feed.sh 0.1.9
```

### 实测

| 场景 | 输出 |
| --- | --- |
| 正常更新 0.1.8 → 0.1.9 | `发现新版本 0.1.9` |
| feed 封禁 0.1.9 | `版本 0.1.9 已被 feed 封禁，跳过` |
| 当前 0.1.9，feed 指向 0.1.8 且允许回滚 | `回滚到 0.1.8` |
| 当前 0.1.8，min_version 0.2.0 | `guard` 退出码 1 |
| 篡改 min_version 不重签 | `策略签名校验失败: 签名不匹配` |

---

## 两个安全实验

### 篡改产物

```bash
printf 'x' >> server/0.1.7/relay-darwin-arm64
./install/relay update
# ✗ 哈希不匹配：得到 ...，期望 ...
```

签名仍然有效，但文件哈希对不上，安装被拒。

### 篡改 feed

```bash
# 把 feed.json 的 version 改成 0.1.8，签名不变
./install/relay update
# ✗ 版本 0.1.8 的签名不匹配，拒绝安装
```

签名覆盖了「版本号 + 清单」，改任何一处都会失败。

恢复：

```bash
bash scripts/make-feed.sh 0.1.7
```

---

## 与真实桌面客户端的差异

| 本实验 | 真实客户端 |
| --- | --- |
| `syscall.Exec` 原地替换进程 | GUI 应用要由脱离的 shell 等旧进程退出后再启动 |
| 只处理 Unix 的 rename | Windows 需要 exe 改名 + 启动检查 + 失败还原 |
| 无界面 | 需要提示、进度、稍后重启 |
| 无灰度与 blocked | 需要版本封禁、最低版本、分阶段放量 |

Tauri 已经内置 `install()` + `relaunch()`，本实验的目的是理解它内部在做什么。

---

## 思考题

1. 为什么签名的是 `SHA256SUMS`，而不是直接签每个文件？
2. 为什么下载要落在可执行文件旁边，而不是 `/tmp`？
3. 如果把签名私钥放进客户端，攻击者能做什么？
4. feed 被完全劫持，但没有私钥，攻击者最多能做到什么？
5. 为什么新进程启动时要删 `.old`，而不是替换完立即删？
