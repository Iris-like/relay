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
func (r *Release) verifyMeta() error {
	if r.Signed == nil {
		return errors.New("feed 没有签名")
	}
	hasPolicy := r.MinVersion != "" || r.Rollback || len(r.Blocked) > 0
	if r.Signed.MetaSig == "" {
		if hasPolicy {
			return errors.New("feed 带策略字段但没有 meta 签名，拒绝使用")
		}
		return nil
	}

	blocked := append([]string(nil), r.Blocked...)
	sort.Strings(blocked)
	msg := fmt.Sprintf(
		"lab02 meta %s\nmin=%s\nrollback=%t\nblocked=%s",
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
}

func statePath() string {
	if p := os.Getenv("LAB02_STATE"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ".lab02-state.json"
	}
	return filepath.Join(dir, "lab02", "state.json")
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
