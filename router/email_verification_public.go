package router

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"realms/internal/crypto"
	emailpkg "realms/internal/email"
)

const (
	emailVerificationCodeMod = 1000000
	emailVerificationTTL     = 10 * time.Minute
)

func emailVerificationSendHandler(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if opts.Store == nil {
			http.Error(w, "store 未初始化", http.StatusInternalServerError)
			return
		}

		enabled, err := emailVerificationEnabled(r.Context(), opts)
		if err != nil {
			http.Error(w, "查询配置失败", http.StatusInternalServerError)
			return
		}
		if !enabled {
			http.Error(w, "当前环境未启用邮箱验证码", http.StatusForbidden)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "表单解析失败", http.StatusBadRequest)
			return
		}

		email := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
		if email == "" {
			http.Error(w, "email 不能为空", http.StatusBadRequest)
			return
		}
		if _, err := mail.ParseAddress(email); err != nil {
			http.Error(w, "email 不合法", http.StatusBadRequest)
			return
		}

		if _, err := opts.Store.GetUserByEmail(r.Context(), email); err == nil {
			http.Error(w, "邮箱地址已被占用", http.StatusBadRequest)
			return
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "查询邮箱失败", http.StatusInternalServerError)
			return
		}

		code, err := newEmailVerificationCode()
		if err != nil {
			http.Error(w, "生成验证码失败", http.StatusInternalServerError)
			return
		}
		expiresAt := time.Now().Add(emailVerificationTTL)

		_, _ = opts.Store.DeleteExpiredEmailVerifications(r.Context())
		if _, err := opts.Store.UpsertEmailVerification(r.Context(), nil, email, crypto.TokenHash(code), expiresAt); err != nil {
			http.Error(w, "写入验证码失败", http.StatusInternalServerError)
			return
		}

		subject := "Realms 邮箱验证码"
		content := fmt.Sprintf("<p>您好，你正在进行 Realms 邮箱验证。</p>"+
			"<p>您的验证码为: <strong>%s</strong></p>"+
			"<p>验证码 10 分钟内有效，如果不是本人操作，请忽略。</p>", code)

		smtpCfg, err := smtpConfigEffective(r.Context(), opts)
		if err != nil {
			http.Error(w, "查询配置失败", http.StatusInternalServerError)
			return
		}
		if strings.TrimSpace(smtpCfg.SMTPServer) == "" || strings.TrimSpace(smtpCfg.SMTPAccount) == "" || strings.TrimSpace(smtpCfg.SMTPToken) == "" {
			http.Error(w, "邮件服务未配置", http.StatusInternalServerError)
			return
		}

		mailer := emailpkg.NewSMTPMailer(smtpCfg)
		if err := mailer.SendHTML(r.Context(), subject, email, content); err != nil {
			http.Error(w, "发送邮件失败", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"sent":true}`))
	}
}

func newEmailVerificationCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(emailVerificationCodeMod))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
