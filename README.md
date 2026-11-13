# Lab 02：自更新

目标：亲手实现「检查 → 下载 → 验签 → 替换 → 重启」，并验证篡改会被拒绝。

不依赖 Tauri、不依赖 GUI，用一个 Go 小程序把机制跑通。

---

## 目录

```
.
├── main.go                  version / check / update 三个命令
├── feed.go                  feed 结构与版本比较
├── verify.go                Ed25519 验签 + 解析签名清单
├── install.go               下载、哈希校验、替换、exec
├── scripts/
│   ├── gen-keys.sh          生成签名密钥
│   ├── build-client.sh      编译带版本号与公钥的客户端
│   └── make-feed.sh         编译新版本、生成 SHA256SUMS、签名、出 feed
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
bash scripts/build-client.sh 0.1.6 install/lab02
./install/lab02 version
# lab02 0.1.6 darwin/arm64
```

公钥会被编译进客户端，私钥只在本地 `.keys/`。

### 3. 生成「新版本」与 feed

```bash
bash scripts/make-feed.sh 0.1.7
```

产物：

```
server/0.1.7/lab02-darwin-arm64
server/0.1.7/lab02-darwin-amd64
server/0.1.7/lab02-linux-amd64
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
./install/lab02 check
# latest 0.1.7, current 0.1.6
# update available

./install/lab02 update
# 发现新版本 0.1.7，下载 lab02-darwin-arm64
# 签名与哈希校验通过，准备替换
# lab02 0.1.7 darwin/arm64

./install/lab02 version
# lab02 0.1.7 darwin/arm64
```

---

## 更新时发生了什么

```
GET /feed.json
  ↓
semver 比较：0.1.7 > 0.1.6
  ↓
校验签名：ed25519("lab02 release 0.1.7\n" + SHA256SUMS)
  ↓
从签名清单里取当前平台产物的 sha256
  ↓
下载到 .lab02.new（与可执行文件同目录）
  ↓
计算 sha256，必须等于清单里的值
  ↓
旧文件改名 lab02.old，新文件改名 lab02
  ↓
syscall.Exec 用新二进制替换当前进程
  ↓
新进程启动时清理 lab02.old
```

两个关键点：

1. **先验签，再信哈希**。哈希来自被签名的清单，不是来自 feed 字段。
2. **暂存文件与目标同目录**，保证 rename 在同一个文件系统上，是原子操作。

---

## 两个安全实验

### 篡改产物

```bash
printf 'x' >> server/0.1.7/lab02-darwin-arm64
./install/lab02 update
# ✗ 哈希不匹配：得到 ...，期望 ...
```

签名仍然有效，但文件哈希对不上，安装被拒。

### 篡改 feed

```bash
# 把 feed.json 的 version 改成 0.1.8，签名不变
./install/lab02 update
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
