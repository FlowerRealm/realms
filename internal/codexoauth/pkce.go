package codexoauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
)

func newRandomHex(bytesLen int) (string, error) {
	if bytesLen < 64 {
		bytesLen = 64
	}
	b := make([]byte, bytesLen)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("生成随机数失败: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func NewPKCE() (verifier string, challenge string, err error) {
	// 对齐 compact gateway：verifier 使用 64 bytes 随机数的 hex 字符串。
	v, err := newRandomHex(64)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(v))
	ch := base64.RawURLEncoding.EncodeToString(sum[:])
	return v, ch, nil
}
