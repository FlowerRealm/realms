package codexoauth

import "strings"

type Config struct {
	ClientID     string
	AuthorizeURL string
	TokenURL     string
	RedirectURI  string
	Scope        string
	Prompt       string
}

const (
	DefaultClientID     = "app_EMoamEEZ73f0CkXaXp7hrann"
	DefaultAuthorizeURL = "https://auth.openai.com/oauth/authorize"
	DefaultTokenURL     = "https://auth.openai.com/oauth/token"
	DefaultRedirectURI  = "http://localhost:1455/auth/callback"
	DefaultScope        = "openid email profile offline_access"
	DefaultPrompt       = ""
)

func DefaultConfig(redirectURI string) Config {
	return Config{
		ClientID:     DefaultClientID,
		AuthorizeURL: DefaultAuthorizeURL,
		TokenURL:     DefaultTokenURL,
		RedirectURI:  strings.TrimSpace(redirectURI),
		Scope:        DefaultScope,
		Prompt:       DefaultPrompt,
	}
}
