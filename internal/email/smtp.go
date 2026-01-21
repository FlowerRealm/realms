// Package email 提供邮件发送能力（当前仅实现 SMTP），供 Web 注册/验证流程使用。
package email

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"realms/internal/config"
)

type Mailer interface {
	SendHTML(ctx context.Context, subject string, to string, html string) error
}

type SMTPMailer struct {
	cfg config.SMTPConfig
}

func NewSMTPMailer(cfg config.SMTPConfig) *SMTPMailer {
	return &SMTPMailer{cfg: cfg}
}

func (m *SMTPMailer) SendHTML(ctx context.Context, subject string, to string, html string) error {
	host := strings.TrimSpace(m.cfg.SMTPServer)
	if host == "" {
		return errors.New("SMTPServer 未配置")
	}
	port := m.cfg.SMTPPort
	if port == 0 {
		port = 587
	}

	from, err := normalizeAddress(firstNonEmpty(m.cfg.SMTPFrom, m.cfg.SMTPAccount))
	if err != nil {
		return fmt.Errorf("SMTP 发件人不合法: %w", err)
	}
	toAddr, err := normalizeAddress(to)
	if err != nil {
		return fmt.Errorf("收件人邮箱不合法: %w", err)
	}

	account := strings.TrimSpace(m.cfg.SMTPAccount)
	token := m.cfg.SMTPToken
	if account == "" || token == "" {
		return errors.New("SMTPAccount/SMTPToken 未配置")
	}

	msg, err := buildHTMLMessage(from, toAddr, subject, html)
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	tlsCfg := &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	}
	deadline := deadlineFromContext(ctx, 30*time.Second)

	if port == 465 || m.cfg.SMTPSSLEnabled {
		return sendMailImplicitTLS(ctx, addr, host, tlsCfg, deadline, account, token, from, []string{toAddr}, msg)
	}
	return sendMailStartTLS(ctx, addr, host, tlsCfg, deadline, account, token, from, []string{toAddr}, msg)
}

func sendMailImplicitTLS(ctx context.Context, addr string, host string, tlsCfg *tls.Config, deadline time.Time, account string, token string, from string, to []string, msg []byte) error {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
	if err != nil {
		return fmt.Errorf("SMTP TLS 连接失败: %w", err)
	}
	if err := conn.SetDeadline(deadline); err != nil {
		_ = conn.Close()
		return fmt.Errorf("设置 SMTP 超时失败: %w", err)
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("创建 SMTP client 失败: %w", err)
	}
	defer func() { _ = c.Close() }()

	return sendWithClient(ctx, c, host, account, token, from, to, msg)
}

func sendMailStartTLS(ctx context.Context, addr string, host string, tlsCfg *tls.Config, deadline time.Time, account string, token string, from string, to []string, msg []byte) error {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("SMTP 连接失败: %w", err)
	}
	if err := conn.SetDeadline(deadline); err != nil {
		_ = conn.Close()
		return fmt.Errorf("设置 SMTP 超时失败: %w", err)
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("创建 SMTP client 失败: %w", err)
	}
	defer func() { _ = c.Close() }()

	ok, _ := c.Extension("STARTTLS")
	if !ok {
		return errors.New("SMTP 服务器不支持 STARTTLS（拒绝在明文连接上 AUTH）")
	}
	if err := c.StartTLS(tlsCfg); err != nil {
		return fmt.Errorf("SMTP STARTTLS 失败: %w", err)
	}
	return sendWithClient(ctx, c, host, account, token, from, to, msg)
}

func sendWithClient(ctx context.Context, c *smtp.Client, host string, account string, token string, from string, to []string, msg []byte) error {
	auth := smtp.PlainAuth("", account, token, host)
	if err := c.Auth(auth); err != nil {
		return fmt.Errorf("SMTP 认证失败: %w", err)
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("SMTP MAIL FROM 失败: %w", err)
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("SMTP RCPT TO 失败: %w", err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA 失败: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return fmt.Errorf("写入 SMTP 内容失败: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("结束 SMTP DATA 失败: %w", err)
	}
	_ = c.Quit()
	return nil
}

func buildHTMLMessage(from string, to string, subject string, html string) ([]byte, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		subject = "Realms 邮件"
	}
	encodedSubject := encodeRFC2047(subject)

	id, err := messageID(from)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	header := fmt.Sprintf("To: %s\r\nFrom: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n",
		to, from, encodedSubject, now.Format(time.RFC1123Z), id)
	body := html
	if !strings.HasSuffix(body, "\r\n") {
		body += "\r\n"
	}
	return []byte(header + body), nil
}

func encodeRFC2047(s string) string {
	return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
}

func messageID(from string) (string, error) {
	parts := strings.Split(from, "@")
	if len(parts) != 2 || parts[1] == "" {
		return "", errors.New("发件人邮箱缺少域名，无法生成 Message-ID")
	}
	randPart, err := randomToken(12)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), randPart, parts[1]), nil
}

func randomToken(n int) (string, error) {
	if n < 8 {
		n = 8
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("生成随机数失败: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func normalizeAddress(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("邮箱为空")
	}
	a, err := mail.ParseAddress(s)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(a.Address), nil
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func deadlineFromContext(ctx context.Context, fallback time.Duration) time.Time {
	if d, ok := ctx.Deadline(); ok {
		return d
	}
	return time.Now().Add(fallback)
}
