#!/usr/bin/env python3
"""从 dist 目录生成 SHA256SUMS、签名和 feed.json

用法: gen-feed.py <version> <asset-base-url> <dist-dir> <private-key> <out-feed>

可选环境变量：
  MIN_VERSION  最低可运行版本
  ROLLBACK     1/true 时允许降级到本版本
  BLOCKED      逗号或空格分隔的封禁版本列表
"""
import hashlib
import json
import os
import subprocess
import sys
import tempfile


def sha256(path: str) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def sign(key: str, data: bytes) -> str:
    # OpenSSL 4 的 Ed25519 一次性签名要求 -in 文件，不能从 stdin 读
    with tempfile.NamedTemporaryFile(delete=False) as tf:
        tf.write(data)
        path = tf.name
    try:
        raw = subprocess.run(
            ["openssl", "pkeyutl", "-sign", "-inkey", key, "-rawin", "-in", path],
            capture_output=True, check=True,
        ).stdout
    finally:
        os.unlink(path)
    return subprocess.run(
        ["openssl", "base64", "-A"], input=raw, capture_output=True, check=True,
    ).stdout.decode().strip()


def main() -> None:
    if len(sys.argv) != 6:
        sys.exit(__doc__)

    version, base, dist, key, out = sys.argv[1:6]
    min_version = os.environ.get("MIN_VERSION", "").strip()
    rollback = os.environ.get("ROLLBACK", "").lower() in ("1", "true", "yes")
    blocked = sorted(
        x for x in os.environ.get("BLOCKED", "").replace(",", " ").split() if x
    )

    names = sorted(n for n in os.listdir(dist) if n.startswith("lab02-"))
    if not names:
        sys.exit(f"{dist} 里没有 lab02-* 产物")

    sums = "\n".join(f"{sha256(os.path.join(dist, n))}  {n}" for n in names)
    sig = sign(key, f"lab02 release {version}\n{sums}".encode())

    meta = (
        f"lab02 meta {version}\n"
        f"min={min_version}\n"
        f"rollback={'true' if rollback else 'false'}\n"
        f"blocked={','.join(blocked)}"
    )
    meta_sig = sign(key, meta.encode())

    with open(os.path.join(dist, "SHA256SUMS"), "w") as f:
        f.write(sums + "\n")
    with open(os.path.join(dist, "SHA256SUMS.sig"), "w") as f:
        f.write(sig + "\n")

    assets = {
        n: {"url": f"{base}/{n}", "sha256": sha256(os.path.join(dist, n))}
        for n in names
    }
    doc = {
        "version": version,
        "notes": "修复若干问题",
        "min_version": min_version,
        "rollback": rollback,
        "blocked": blocked,
        "assets": assets,
        "signed": {"sums": sums, "sig": sig, "meta_sig": meta_sig},
    }

    os.makedirs(os.path.dirname(out) or ".", exist_ok=True)
    with open(out, "w") as f:
        json.dump(doc, f, ensure_ascii=False, indent=2)
    print(f"feed: {out} ({len(names)} assets, version {version})")


if __name__ == "__main__":
    main()
