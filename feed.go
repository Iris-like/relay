package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Asset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

type Signed struct {
	Sums    string `json:"sums"`
	Sig     string `json:"sig"`
	MetaSig string `json:"meta_sig,omitempty"`
}

type Release struct {
	Version    string           `json:"version"`
	Notes      string           `json:"notes"`
	MinVersion string           `json:"min_version,omitempty"`
	Rollback   bool             `json:"rollback,omitempty"`
	Blocked    []string         `json:"blocked,omitempty"`
	Assets     map[string]Asset `json:"assets"`
	Signed     *Signed          `json:"signed"`
}

func fetchRelease(url string) (*Release, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	var rel Release
	if err := json.NewDecoder(res.Body).Decode(&rel); err != nil {
		return nil, err
	}
	if rel.Version == "" {
		return nil, fmt.Errorf("feed 里没有 version")
	}
	return &rel, nil
}

// Newer 判断 a 是否比 b 新；主版本相同则正式版高于预发布版，
// 预发布之间按点分标识符比较（数字段按数值、其余按字典序，缺段者更小）。
func Newer(a, b string) bool {
	av, bv := parseVer(a), parseVer(b)
	for i := 0; i < 3; i++ {
		if av.num[i] != bv.num[i] {
			return av.num[i] > bv.num[i]
		}
	}
	// 主版本号相同：无预发布后缀 > 有预发布后缀
	if len(av.pre) == 0 || len(bv.pre) == 0 {
		return len(av.pre) == 0 && len(bv.pre) > 0
	}
	for i := 0; i < len(av.pre) && i < len(bv.pre); i++ {
		an, aerr := strconv.Atoi(av.pre[i])
		bn, berr := strconv.Atoi(bv.pre[i])
		if aerr == nil && berr == nil {
			if an != bn {
				return an > bn
			}
		} else if av.pre[i] != bv.pre[i] {
			return av.pre[i] > bv.pre[i]
		}
	}
	return len(av.pre) > len(bv.pre)
}

type parsedVersion struct {
	num [3]int
	pre []string
}

func parseVer(v string) parsedVersion {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	var out parsedVersion
	// 先按 - 拆出预发布部分；+ 之后的构建元数据不参与比较
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	if i := strings.IndexByte(v, '-'); i >= 0 {
		out.pre = strings.Split(v[i+1:], ".")
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	for i := 0; i < 3 && i < len(parts); i++ {
		out.num[i], _ = strconv.Atoi(parts[i])
	}
	return out
}
