#!/usr/bin/env bash
set -euo pipefail

KEY_DIR=".keys"
KEY_NAME="update-key.pem"
PUB_NAME="update-key.pub.pem"
mkdir -p "$KEY_DIR"
KEY_FILE="$KEY_DIR/$KEY_NAME"
PUB_FILE="$KEY_DIR/$PUB_NAME"

if [ -f "$KEY_FILE" ]; then
  echo "密钥已存在：$KEY_FILE"
  exit 0
fi

openssl genpkey -algorithm ED25519 -out "$KEY_FILE"
openssl pkey -in "$KEY_FILE" -pubout -out "$PUB_FILE"
chmod 600 "$KEY_FILE"
echo "已生成："
echo "  私钥 $KEY_FILE"
echo "  公钥 $PUB_FILE"
