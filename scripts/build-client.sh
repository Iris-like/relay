#!/usr/bin/env bash
# 编译一个带版本号和公钥的客户端
set -euo pipefail

VERSION="${1:?用法: scripts/build-client.sh 0.1.6 install/relay}"
OUT="${2:?缺少输出路径}"

KEY_DIR=".keys"
KEY_NAME="update-key.pem"
KEY="${KEY_DIR}/${KEY_NAME}"
[ -f "$KEY" ] || { echo "缺少私钥，先执行 scripts/gen-keys.sh"; exit 1; }

PUB=$(openssl pkey -in "$KEY" -pubout -outform DER | tail -c 32 | base64)

mkdir -p "$(dirname "$OUT")"
CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X main.version=${VERSION} -X main.signingKey=${PUB}" \
  -o "$OUT" .

echo "客户端 ${VERSION} -> ${OUT}"
