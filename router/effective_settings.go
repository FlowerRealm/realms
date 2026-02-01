package router

import (
	"context"

	"realms/internal/config"
	"realms/internal/store"
)

func emailVerificationEnabled(ctx context.Context, opts Options) (bool, error) {
	enabled := opts.EmailVerificationEnabledDefault
	if opts.Store == nil {
		return enabled, nil
	}
	v, ok, err := opts.Store.GetBoolAppSetting(ctx, store.SettingEmailVerificationEnable)
	if err != nil {
		return false, err
	}
	if ok {
		enabled = v
	}
	return enabled, nil
}

func smtpConfigEffective(ctx context.Context, opts Options) (config.SMTPConfig, error) {
	out := opts.SMTPDefault
	if out.SMTPPort == 0 {
		out.SMTPPort = 587
	}
	if opts.Store == nil {
		return out, nil
	}

	smtpServer, smtpServerOK, err := opts.Store.GetStringAppSetting(ctx, store.SettingSMTPServer)
	if err != nil {
		return config.SMTPConfig{}, err
	}
	if smtpServerOK {
		out.SMTPServer = smtpServer
	}

	smtpPort, smtpPortOK, err := opts.Store.GetIntAppSetting(ctx, store.SettingSMTPPort)
	if err != nil {
		return config.SMTPConfig{}, err
	}
	if smtpPortOK {
		out.SMTPPort = smtpPort
	}
	if out.SMTPPort == 0 {
		out.SMTPPort = 587
	}

	smtpSSL, smtpSSLOK, err := opts.Store.GetBoolAppSetting(ctx, store.SettingSMTPSSLEnabled)
	if err != nil {
		return config.SMTPConfig{}, err
	}
	if smtpSSLOK {
		out.SMTPSSLEnabled = smtpSSL
	}

	smtpAccount, smtpAccountOK, err := opts.Store.GetStringAppSetting(ctx, store.SettingSMTPAccount)
	if err != nil {
		return config.SMTPConfig{}, err
	}
	if smtpAccountOK {
		out.SMTPAccount = smtpAccount
	}

	smtpFrom, smtpFromOK, err := opts.Store.GetStringAppSetting(ctx, store.SettingSMTPFrom)
	if err != nil {
		return config.SMTPConfig{}, err
	}
	if smtpFromOK {
		out.SMTPFrom = smtpFrom
	}

	smtpToken, smtpTokenOK, err := opts.Store.GetStringAppSetting(ctx, store.SettingSMTPToken)
	if err != nil {
		return config.SMTPConfig{}, err
	}
	if smtpTokenOK {
		out.SMTPToken = smtpToken
	}

	return out, nil
}
