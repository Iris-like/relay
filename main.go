package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// 构建时注入
var version = "dev"
var signingKey = "" // Ed25519 公钥，base64（raw 32 字节）

func main() {
	cleanupOld()

	args := os.Args[1:]
	cmd := "version"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd = args[0]
		args = args[1:]
	}

	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	feed := fs.String("feed", "http://127.0.0.1:8099/feed.json", "更新 feed 地址")
	_ = fs.Parse(args)

	switch cmd {
	case "version":
		fmt.Printf("lab02 %s %s/%s\n", version, runtime.GOOS, runtime.GOARCH)

	case "check":
		rel, err := fetchRelease(*feed)
		if err != nil {
			fail("读取 feed 失败: %v", err)
		}
		fmt.Printf("latest %s, current %s\n", rel.Version, version)
		if Newer(rel.Version, version) {
			fmt.Println("update available")
		} else {
			fmt.Println("up to date")
		}

	case "update":
		rel, err := fetchRelease(*feed)
		if err != nil {
			fail("读取 feed 失败: %v", err)
		}
		if !Newer(rel.Version, version) {
			fmt.Printf("已是最新版本 %s\n", version)
			return
		}
		name := assetName()
		asset, ok := rel.Assets[name]
		if !ok {
			fail("feed 里没有当前平台的产物 %s", name)
		}
		fmt.Printf("发现新版本 %s，下载 %s\n", rel.Version, name)
		staged, err := downloadAndVerify(rel, name, asset)
		if err != nil {
			fail("下载或校验失败: %v", err)
		}
		fmt.Println("签名与哈希校验通过，准备替换")
		if err := replaceAndRelaunch(staged); err != nil {
			fail("替换失败: %v", err)
		}

	default:
		fmt.Fprintf(os.Stderr, "用法: lab02 [version|check|update] [--feed URL]\n")
		os.Exit(2)
	}
}

func assetName() string {
	return fmt.Sprintf("lab02-%s-%s", runtime.GOOS, runtime.GOARCH)
}

// cleanupOld 删除上一次更新留下的旧二进制
func cleanupOld() {
	exe, err := executable()
	if err != nil {
		return
	}
	os.Remove(exe + ".old")
}

func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "✗ "+format+"\n", args...)
	os.Exit(1)
}
