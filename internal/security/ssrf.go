// Package security 提供最小的 base_url 校验：避免明显的配置错误（协议/Host/DNS）。
package security

import (
	"errors"
	"fmt"
	"net"
	"net/url"
)

func ValidateBaseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("解析 base_url 失败: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("base_url 仅支持 http/https")
	}
	if u.Host == "" {
		return nil, errors.New("base_url host 不能为空")
	}
	host := u.Hostname()
	if host == "" {
		return nil, errors.New("base_url host 不能为空")
	}
	if ip := net.ParseIP(host); ip != nil {
		return u, nil
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, fmt.Errorf("解析 base_url DNS 失败: %w", err)
	}
	if len(ips) == 0 {
		return nil, errors.New("base_url 无可用 DNS 解析结果")
	}
	return u, nil
}
