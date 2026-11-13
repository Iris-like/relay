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
	Sums string `json:"sums"`
	Sig  string `json:"sig"`
}

type Release struct {
	Version string           `json:"version"`
	Notes   string           `json:"notes"`
	Assets  map[string]Asset `json:"assets"`
	Signed  *Signed          `json:"signed"`
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

// Newer 判断 a 是否比 b 新；只比较数字段，遇到非数字按 0 处理
func Newer(a, b string) bool {
	as, bs := parseVer(a), parseVer(b)
	for i := 0; i < 3; i++ {
		if as[i] != bs[i] {
			return as[i] > bs[i]
		}
	}
	return false
}

func parseVer(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	// 去掉 -beta 之类的后缀
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	var out [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		out[i], _ = strconv.Atoi(parts[i])
	}
	return out
}
