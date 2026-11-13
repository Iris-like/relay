package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

func publicKey() (ed25519.PublicKey, error) {
	if signingKey == "" {
		return nil, errors.New("客户端没有内置公钥")
	}
	pub, err := base64.StdEncoding.DecodeString(signingKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return nil, errors.New("内置公钥无效")
	}
	return pub, nil
}

func verifySig(msg, sigB64 string) error {
	pub, err := publicKey()
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sigB64))
	if err != nil || len(sig) != ed25519.SignatureSize {
		return errors.New("签名格式无效")
	}
	if !ed25519.Verify(pub, []byte(msg), sig) {
		return errors.New("签名不匹配")
	}
	return nil
}

// verifySums 用内置公钥验证签名，返回被签名背书的 文件名 -> sha256
func (r *Release) verifySums() (map[string]string, error) {
	if r.Signed == nil {
		return nil, errors.New("feed 没有签名，拒绝安装")
	}
	if err := verifySig("lab02 release "+r.Version+"\n"+r.Signed.Sums, r.Signed.Sig); err != nil {
		return nil, fmt.Errorf("版本 %s 的签名无效，拒绝安装: %w", r.Version, err)
	}

	sums := map[string]string{}
	for _, line := range strings.Split(r.Signed.Sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if b, err := hex.DecodeString(fields[0]); err != nil || len(b) != 32 {
			continue
		}
		sums[strings.TrimPrefix(fields[1], "*")] = strings.ToLower(fields[0])
	}
	return sums, nil
}
