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
		fmt.Printf("relay %s %s/%s\n", version, runtime.GOOS, runtime.GOARCH)

	case "check":
		doCheck(*feed)

	case "update":
		doUpdate(*feed)

	case "guard":
		doGuard(*feed)

	case "block":
		v := fs.Arg(0)
		if v == "" {
			fail("用法: relay block <version>")
		}
		if err := blockLocally(v); err != nil {
			fail("写入状态失败: %v", err)
		}
		fmt.Printf("已封禁 %s（仅本机）\n", v)

	case "unblock":
		v := fs.Arg(0)
		if v == "" {
			fail("用法: relay unblock <version>")
		}
		if err := unblockLocally(v); err != nil {
			fail("写入状态失败: %v", err)
		}
		fmt.Printf("已解封 %s\n", v)

	case "blocked":
		s := loadState()
		if len(s.Blocked) == 0 {
			fmt.Println("没有本机封禁的版本")
			return
		}
		for _, v := range s.Blocked {
			fmt.Println(v)
		}

	default:
		fmt.Fprintln(os.Stderr, "用法: relay [version|check|update|guard|block|unblock|blocked] [--feed URL]")
		os.Exit(2)
	}
}

func doCheck(feed string) {
	rel, err := fetchRelease(feed)
	if err != nil {
		fail("读取 feed 失败: %v", err)
	}
	if err := rel.verifyMeta(); err != nil {
		fail("策略签名校验失败: %v", err)
	}
	rememberMinVersion(rel.MinVersion)

	fmt.Printf("latest %s, current %s\n", rel.Version, version)
	if rel.MinVersion != "" {
		fmt.Printf("min_version %s\n", rel.MinVersion)
	}

	if contains(rel.Blocked, rel.Version) {
		fmt.Println("最新版本已被 feed 封禁，不会更新")
		return
	}
	if blockedLocally(rel.Version) {
		fmt.Println("最新版本已被本机封禁，不会更新")
		return
	}

	switch {
	case Newer(rel.Version, version):
		fmt.Println("update available")
		if Newer(rel.MinVersion, version) {
			fmt.Println("强制更新：当前版本低于最低支持版本")
		}
	case Newer(version, rel.Version):
		if rel.Rollback {
			fmt.Println("rollback available")
		} else {
			fmt.Println("feed 指向旧版本，但未允许回滚")
		}
	default:
		fmt.Println("up to date")
	}
}

func doUpdate(feed string) {
	rel, err := fetchRelease(feed)
	if err != nil {
		fail("读取 feed 失败: %v", err)
	}
	if err := rel.verifyMeta(); err != nil {
		fail("策略签名校验失败: %v", err)
	}
	rememberMinVersion(rel.MinVersion)

	cur, target := version, rel.Version

	if contains(rel.Blocked, target) {
		fmt.Printf("版本 %s 已被 feed 封禁，跳过\n", target)
		return
	}
	if blockedLocally(target) {
		fmt.Printf("版本 %s 已被本机封禁，跳过\n", target)
		return
	}
	if Newer(rel.MinVersion, target) {
		fail("feed 无效：min_version %s 高于目标版本 %s", rel.MinVersion, target)
	}

	downgrade := Newer(cur, target)
	if downgrade && !rel.Rollback {
		fmt.Printf("feed 指向旧版本 %s，但未允许回滚\n", target)
		return
	}
	if target == cur {
		fmt.Printf("已是最新版本 %s\n", cur)
		return
	}
	if downgrade {
		fmt.Printf("回滚到 %s\n", target)
	}

	name := assetName()
	asset, ok := rel.Assets[name]
	if !ok {
		fail("feed 里没有当前平台的产物 %s", name)
	}
	if !downgrade {
		fmt.Printf("发现新版本 %s，下载 %s\n", target, name)
	}
	staged, err := downloadAndVerify(rel, name, asset)
	if err != nil {
		fail("下载或校验失败: %v", err)
	}
	fmt.Println("签名与哈希校验通过，准备替换")
	if err := replaceAndRelaunch(staged); err != nil {
		fail("替换失败: %v", err)
	}
}

// doGuard 供启动时调用：低于最低支持版本直接拒绝运行。
// 读不到 feed 是网络问题，不是版本问题，输出 warning 并放行，
// 避免断网时应用被自己挡住；同时用缓存的 min_version 继续兜底。
func doGuard(feed string) {
	rel, err := fetchRelease(feed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠ 读取 feed 失败，跳过在线版本检查：%v\n", err)
	} else if verr := rel.verifyMeta(); verr != nil {
		// 验签失败属于 feed 不可信：不让它影响启动，但也不能采信其中的 min_version
		fmt.Fprintf(os.Stderr, "⚠ 策略签名校验失败，跳过在线版本检查：%v\n", verr)
	} else {
		rememberMinVersion(rel.MinVersion)
		if Newer(rel.MinVersion, version) {
			fmt.Fprintf(os.Stderr, "当前 %s 低于最低支持版本 %s，必须更新后使用\n", version, rel.MinVersion)
			os.Exit(1)
		}
		confirmStart()
		fmt.Println("ok")
		return
	}

	// 网络失败或验签失败时，仍用上次成功读到的最低版本兜底
	s := loadState()
	if Newer(s.LastMinVersion, version) {
		fmt.Fprintf(os.Stderr, "当前 %s 低于缓存的最低支持版本 %s，必须更新后使用\n", version, s.LastMinVersion)
		os.Exit(1)
	}
	confirmStart()
	fmt.Println("ok（离线）")
}

// confirmStart 只在 guard 通过后计数：读版本、更新、封禁这些命令
// 不是应用启动，安装时 exec 的 version 健康检查也不该算一次成功启动。
func confirmStart() {
	noteSuccessfulStart(version)
	cleanupOld()
}

func assetName() string {
	return fmt.Sprintf("relay-%s-%s", runtime.GOOS, runtime.GOARCH)
}

// cleanupOld 在新版本被确认可用后才删除上一次更新留下的旧二进制。
// 计数只来自 guard 成功（见 confirmStart），连续 N 次启动检查通过才清理，
// 这样安装时 exec 的 `version` 健康检查不会被当成一次成功启动。
const startsBeforeCleanup = 2

// 测试时替换成固定路径，避免依赖运行测试的进程可执行文件。
var executablePath = executable

func cleanupOld() {
	exe, err := executablePath()
	if err != nil {
		return
	}
	backup := exe + ".old"
	if _, err := os.Stat(backup); err != nil {
		return
	}
	s := loadState()
	if s.ConfirmedVersion != version || s.Starts < startsBeforeCleanup {
		return
	}
	os.Remove(backup)
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
