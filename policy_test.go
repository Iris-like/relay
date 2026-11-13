package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"os/exec"
	"testing"
)

// 用临时密钥对签名，构造合法/被篡改的 feed，验证策略签名不可被绕过。
func withTestKey(t *testing.T) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	old := signingKey
	signingKey = base64.StdEncoding.EncodeToString(pub)
	t.Cleanup(func() { signingKey = old })
	t.Setenv("RELAY_STATE", t.TempDir()+"/state.json")
	signMeta = func(msg string) string {
		return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(msg)))
	}
}

var signMeta func(string) string

func signedRelease(t *testing.T, min string, rollback bool, blocked []string) *Release {
	t.Helper()
	msg := "relay meta 0.4.0\nmin=" + min +
		"\nrollback=" + boolText(rollback) +
		"\nblocked=" + joinSorted(blocked)
	return &Release{
		Version:    "0.4.0",
		MinVersion: min,
		Rollback:   rollback,
		Blocked:    blocked,
		Assets:     map[string]Asset{},
		Signed:     &Signed{Sums: "x", Sig: "x", MetaSig: signMeta(msg)},
	}
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func joinSorted(in []string) string {
	out := append([]string(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	s := ""
	for i, v := range out {
		if i > 0 {
			s += ","
		}
		s += v
	}
	return s
}

// 对应 issue #2 第 2 条：策略字段连同 meta_sig 一起删掉，必须被拒绝
func TestVerifyMetaRequiresSignature(t *testing.T) {
	withTestKey(t)

	// 策略为空时也要有签名：合法
	if err := signedRelease(t, "", false, nil).verifyMeta(); err != nil {
		t.Fatalf("带签名的空策略应通过: %v", err)
	}

	// 剥离全部策略字段 + meta_sig，必须拒绝
	bare := &Release{Version: "0.4.0", Signed: &Signed{Sums: "x", Sig: "x"}}
	if err := bare.verifyMeta(); err == nil {
		t.Fatal("剥离 meta_sig 的 feed 竟然通过了校验")
	}

	// 只删 meta_sig，保留策略字段，也必须拒绝
	noSig := signedRelease(t, "0.5.0", false, []string{"0.3.0"})
	noSig.Signed.MetaSig = ""
	if err := noSig.verifyMeta(); err == nil {
		t.Fatal("无 meta_sig 的策略 feed 竟然通过了校验")
	}

	// 篡改任意一个策略字段，验签必须失败
	tampered := signedRelease(t, "0.5.0", false, nil)
	tampered.MinVersion = "0.0.1"
	if err := tampered.verifyMeta(); err == nil {
		t.Fatal("篡改 min_version 后验签竟然通过")
	}
	tampered2 := signedRelease(t, "", false, nil)
	tampered2.Rollback = true
	if err := tampered2.verifyMeta(); err == nil {
		t.Fatal("篡改 rollback 后验签竟然通过")
	}
	tampered3 := signedRelease(t, "", false, nil)
	tampered3.Blocked = []string{"0.3.0"}
	if err := tampered3.verifyMeta(); err == nil {
		t.Fatal("篡改 blocked 后验签竟然通过")
	}
}

// 对应 issue #2 第 1 条：断网时 guard 放行，但缓存的 min_version 仍生效
func TestGuardOfflineUsesCachedMinVersion(t *testing.T) {
	withTestKey(t)
	p := os.Getenv("RELAY_STATE")

	// 模拟曾经成功读到 min_version=0.5.0
	rememberMinVersion("0.5.0")
	s := loadState()
	if s.LastMinVersion != "0.5.0" {
		t.Fatalf("缓存写入失败: %+v", s)
	}
	// 缓存生效时 0.4.0 仍会被拦截（由 doGuard 中的 Newer 判断覆盖）
	if !Newer(s.LastMinVersion, "0.4.0") {
		t.Fatal("缓存的最低版本应高于当前版本")
	}
	// 缓存为空时不拦截
	_ = os.WriteFile(p, []byte("{}"), 0o644)
	if Newer(loadState().LastMinVersion, "0.4.0") {
		t.Fatal("空缓存不应拦截")
	}

	// 端到端：有缓存时断网 guard 仍拦截；无缓存时断网 guard 放行
	rememberMinVersion("0.5.0")
	cached := cmdResult(t, "guard", "--feed", "http://127.0.0.1:1/feed.json")
	if cached.code != 1 {
		t.Fatalf("有缓存时断网 guard 应退出 1，实际 %d (%s)", cached.code, cached.out)
	}
	_ = os.WriteFile(p, []byte("{}"), 0o644)
	fresh := cmdResult(t, "guard", "--feed", "http://127.0.0.1:1/feed.json")
	if fresh.code != 0 {
		t.Fatalf("无缓存时断网 guard 应放行，实际 %d (%s)", fresh.code, fresh.out)
	}
}

type cmdOut struct {
	code int
	out  string
}

// cmdResult 在子进程里跑真实的 doGuard（它会 os.Exit，不能在测试进程内直接调）。
func cmdResult(t *testing.T, args ...string) cmdOut {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=TestHelperProcess")
	cmd.Env = append(os.Environ(),
		"RELAY_HELPER=1",
		"RELAY_HELPER_FEED="+args[len(args)-1],
		"RELAY_STATE="+os.Getenv("RELAY_STATE"),
	)
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("运行子命令失败: %v", err)
	}
	return cmdOut{code: code, out: string(out)}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("RELAY_HELPER") != "1" {
		return
	}
	doGuard(os.Getenv("RELAY_HELPER_FEED"))
	os.Exit(0)
}

// 对应 issue #2 第 4 条：连续启动次数达阈值前保留 .old
func TestCleanupOldWaitsForConfirmedStarts(t *testing.T) {
	withTestKey(t)
	oldVersion := version
	version = "0.4.1"
	t.Cleanup(func() { version = oldVersion })

	dir := t.TempDir()
	exe := dir + "/relay"
	backup := exe + ".old"
	if err := os.WriteFile(backup, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := executablePath
	executablePath = func() (string, error) { return exe, nil }
	t.Cleanup(func() { executablePath = old })

	// 第一次启动：计数 1，备份应保留
	noteSuccessfulStart(version)
	cleanupOld()
	if _, err := os.Stat(backup); err != nil {
		t.Fatal("首次启动后备份不应被删除")
	}
	// 第二次启动：计数 2，备份可以清理
	noteSuccessfulStart(version)
	cleanupOld()
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatal("连续启动成功后备份应被清理")
	}
}
