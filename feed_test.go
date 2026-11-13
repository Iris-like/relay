package main

import "testing"

// 对应 issue #2 第 5 条：预发布版本不应被当成正式版
func TestNewerPrerelease(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.5.0", "0.5.0-beta.1", true},  // 正式版高于同主版本号的 beta
		{"0.5.0-beta.1", "0.5.0", false}, // 反向不更新
		{"0.5.0-beta.2", "0.5.0-beta.1", true},
		{"0.5.0-beta.1", "0.5.0-beta.2", false},
		{"0.5.0-beta.10", "0.5.0-beta.2", true}, // 数字标识符按数值比
		{"0.5.1", "0.5.0", true},
		{"0.5.0", "0.5.0", false},
		{"v0.5.0+build.1", "0.5.0", false}, // + 构建元数据不参与比较
		{"0.4.9", "0.5.0-beta.1", false},   // 主版本号优先
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
