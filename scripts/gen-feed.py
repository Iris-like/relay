#!/usr/bin/env python3
"""从 dist 目录生成 SHA256SUMS、签名和 feed.json

用法: gen-feed.py <version> <asset-base-url> <dist-dir> <private-key> <out-feed>
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


def main() -> None:
    if len(sys.argv) != 6:
        sys.exit(__doc__)

    version, base, dist, key, out = sys.argv[1:6]
    names = sorted(n for n in os.listdir(dist) if n.startswith("lab02-"))
    if not names:
        sys.exit(f"{dist} 里没有 lab02-* 产物")

    sums = "\n".join(f"{sha256(os.path.join(dist, n))}  {n}" for n in names)
    msg = f"lab02 release {version}\n{sums}".encode()

    # OpenSSL 4 的 Ed25519 一次性签名要求 -in 文件，不能从 stdin 读
    with tempfile.NamedTemporaryFile(delete=False) as tf:
        tf.write(msg)
        msg_path = tf.name
    try:
        sig_bin = subprocess.run(
            ["openssl", "pkeyutl", "-sign", "-inkey", key, "-rawin", "-in", msg_path],
            capture_output=True, check=True,
        ).stdout
    finally:
        os.unlink(msg_path)
    sig = subprocess.run(
        ["openssl", "base64", "-A"], input=sig_bin, capture_output=True, check=True,
    ).stdout.decode().strip()

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
        "assets": assets,
        "signed": {"sums": sums, "sig": sig},
    }

    os.makedirs(os.path.dirname(out) or ".", exist_ok=True)
    with open(out, "w") as f:
        json.dump(doc, f, ensure_ascii=False, indent=2)
    print(f"feed: {out} ({len(names)} assets, version {version})")


if __name__ == "__main__":
    main()
