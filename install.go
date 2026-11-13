package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func downloadAndVerify(rel *Release, name string, asset Asset) (string, error) {
	sums, err := rel.verifySums()
	if err != nil {
		return "", err
	}
	want, ok := sums[name]
	if !ok {
		return "", fmt.Errorf("签名清单里没有 %s", name)
	}

	exe, err := executable()
	if err != nil {
		return "", err
	}
	staged := filepath.Join(filepath.Dir(exe), "."+filepath.Base(exe)+".new")
	os.Remove(staged)

	if err := download(asset.URL, staged); err != nil {
		os.Remove(staged)
		return "", err
	}

	got, err := sha256File(staged)
	if err != nil {
		os.Remove(staged)
		return "", err
	}
	if !strings.EqualFold(got, want) {
		os.Remove(staged)
		return "", fmt.Errorf("哈希不匹配：得到 %s，期望 %s", got, want)
	}

	if err := os.Chmod(staged, 0o755); err != nil {
		os.Remove(staged)
		return "", err
	}
	return staged, nil
}

func download(url, path string) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	res, err := client.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("下载 %s 失败：HTTP %d", url, res.StatusCode)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, res.Body)
	return err
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// replaceAndRelaunch 把新文件换到当前可执行文件的位置，然后 exec 自己。
// 成功时进程映像被新版本替换，不会返回。
func replaceAndRelaunch(staged string) error {
	exe, err := executable()
	if err != nil {
		return err
	}
	backup := exe + ".old"
	os.Remove(backup)

	if err := os.Rename(exe, backup); err != nil {
		return err
	}
	if err := os.Rename(staged, exe); err != nil {
		os.Rename(backup, exe)
		return err
	}

	// 用新二进制替换当前进程映像，附带 version 参数验证启动
	if err := syscall.Exec(exe, []string{exe, "version"}, os.Environ()); err != nil {
		os.Rename(exe, staged)
		os.Rename(backup, exe)
		return err
	}
	return nil
}
