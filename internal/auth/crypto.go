// Package auth 提供用户密码与随机 Token 的生成逻辑，避免在 handler 中重复实现安全细节。
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) ([]byte, error) {
	if len(password) < 8 {
		return nil, fmt.Errorf("密码长度至少 8 位")
	}
	return bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
}

func CheckPassword(hash []byte, password string) bool {
	return bcrypt.CompareHashAndPassword(hash, []byte(password)) == nil
}

func NewRandomToken(prefix string, bytesLen int) (string, error) {
	if bytesLen < 16 {
		bytesLen = 16
	}
	b := make([]byte, bytesLen)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("生成随机数失败: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b), nil
}
