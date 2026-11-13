#!/usr/bin/env bash
# 编译新版本，并生成 SHA256SUMS、签名和 feed.json
# 可选环境变量：MIN_VERSION=0.2.0 / ROLLBACK=1 / BLOCKED="0.1.9 0.1.10"
set -euo pipefail

VERSION="${1:?用法: scripts/make-feed.sh 0.1.7 [base-url]}"
BASE="${2:-http://127.0.0.1:8099}"

KEY_DIR=".keys"
KEY_NAME="update-key.pem"
KEY="${KEY_DIR}/${KEY_NAME}"
[ -f "$KEY" ] || { echo "缺少私钥，先执行 scripts/gen-keys.sh"; exit 1; }

PUB=$(openssl pkey -in "$KEY" -pubout -outform DER | tail -c 32 | base64)

DIR="server/${VERSION}"
mkdir -p "$DIR"

for target in darwin/arm64 darwin/amd64 linux/amd64; do
  os=${target%/*}
  arch=${target#*/}
  echo "  build ${os}/${arch}"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.signingKey=${PUB}" \
    -o "${DIR}/relay-${os}-${arch}" .
done

python3 scripts/gen-feed.py "$VERSION" "${BASE}/${VERSION}" "$DIR" "$KEY" "server/feed.json"
