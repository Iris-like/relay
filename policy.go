package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// verifyMeta 校验策略字段（最低版本、回滚、封禁列表）的签名。
// 这些字段不在 SHA256SUMS 里，必须单独签名，否则 feed 可以随意篡改。
//
// 策略字段与 meta_sig 必须成对出现：即使三个字段全为空，也要求有签名。
// 否则攻击者可以把 min_version / blocked / rollback 连同 meta_sig 一起删掉，
// 旧代码会因为「没有策略字段」直接放行，等于绕过了强制更新与封禁策略。
func (r *Release) verifyMeta() error {
	if r.Signed == nil {
		return errors.New("feed 没有签名")
	}
	if r.Signed.MetaSig == "" {
		return errors.New("feed 缺少 meta 签名，拒绝使用")
	}

	blocked := append([]string(nil), r.Blocked...)
	sort.Strings(blocked)
	msg := fmt.Sprintf(
		"relay meta %s\nmin=%s\nrollback=%t\nblocked=%s",
		r.Version, r.MinVersion, r.Rollback, strings.Join(blocked, ","),
	)
	return verifySig(msg, r.Signed.MetaSig)
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

type state struct {
	Blocked []string `json:"blocked"`
	// 最近一次成功读取到的 min_version，用于断网时仍执行强制更新策略
	LastMinVersion string `json:"last_min_version,omitempty"`
	// 当前版本连续成功启动的次数，用于决定何时清理 .old 备份
	Starts int `json:"starts,omitempty"`
	// 已经确认可用的版本，版本变化后重新计数
	ConfirmedVersion string `json:"confirmed_version,omitempty"`
}

func statePath() string {
	if p := os.Getenv("RELAY_STATE"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ".relay-state.json"
	}
	return filepath.Join(dir, "relay", "state.json")
}

func loadState() state {
	var s state
	data, err := os.ReadFile(statePath())
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s)
	return s
}

func saveState(s state) error {
	p := statePath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

func blockedLocally(v string) bool {
	return contains(loadState().Blocked, v)
}

func blockLocally(v string) error {
	s := loadState()
	if !contains(s.Blocked, v) {
		s.Blocked = append(s.Blocked, v)
		sort.Strings(s.Blocked)
	}
	return saveState(s)
}

func unblockLocally(v string) error {
	s := loadState()
	out := s.Blocked[:0]
	for _, item := range s.Blocked {
		if item != v {
			out = append(out, item)
		}
	}
	s.Blocked = out
	return saveState(s)
}

// rememberMinVersion 记录本次成功读到的 feed 中的最低版本。
func rememberMinVersion(v string) {
	s := loadState()
	if s.LastMinVersion == v {
		return
	}
	s.LastMinVersion = v
	_ = saveState(s)
}

// noteSuccessfulStart 累计当前版本的成功启动次数；版本变化时重新计数。
func noteSuccessfulStart(v string) {
	s := loadState()
	if s.ConfirmedVersion != v {
		s.ConfirmedVersion = v
		s.Starts = 1
	} else {
		s.Starts++
	}
	_ = saveState(s)
}
