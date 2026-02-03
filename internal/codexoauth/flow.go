package codexoauth

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	gorillasessions "github.com/gorilla/sessions"

	"realms/internal/auth"
	"realms/internal/config"
	"realms/internal/store"
)

type Pending struct {
	EndpointID   int64
	ActorUserID  int64
	CodeVerifier string
	CreatedAt    time.Time
}

type Flow struct {
	st          *store.Store
	client      *Client
	cookieName  string
	cookieStore *gorillasessions.CookieStore

	returnBaseURL string

	mu      sync.Mutex
	pending map[string]Pending
	ttl     time.Duration
}

func NewFlow(st *store.Store, cookieName string, sessionSecret string, returnBaseURL string, redirectURI string) *Flow {
	secret := strings.TrimSpace(sessionSecret)
	var cookieStore *gorillasessions.CookieStore
	if secret != "" {
		cookieStore = gorillasessions.NewCookieStore([]byte(secret))
	}

	redirectURI = strings.TrimSpace(redirectURI)
	return &Flow{
		st:            st,
		client:        NewClient(DefaultConfig(redirectURI)),
		cookieName:    cookieName,
		cookieStore:   cookieStore,
		returnBaseURL: strings.TrimRight(returnBaseURL, "/"),
		pending:       make(map[string]Pending),
		ttl:           30 * time.Minute,
	}
}

func (f *Flow) Start(ctx context.Context, endpointID int64, actorUserID int64) (string, error) {
	state, err := auth.NewRandomToken("oauth_", 32)
	if err != nil {
		return "", Wrap(ErrStateGenerationFailed, err)
	}
	verifier, challenge, err := NewPKCE()
	if err != nil {
		return "", Wrap(ErrPKCEGenerationFailed, err)
	}

	now := time.Now()
	if err := f.pruneExpiredPending(ctx, now); err != nil {
		return "", Wrap(ErrAuthorizeURLFailed, err)
	}
	if err := f.putPending(ctx, state, Pending{
		EndpointID:   endpointID,
		ActorUserID:  actorUserID,
		CodeVerifier: verifier,
		CreatedAt:    now,
	}); err != nil {
		return "", Wrap(ErrAuthorizeURLFailed, err)
	}

	authURL, err := f.client.BuildAuthorizeURL(state, challenge)
	if err != nil {
		return "", Wrap(ErrAuthorizeURLFailed, err)
	}
	return authURL, nil
}

func (f *Flow) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth/callback", f.handleCallback)
	return mux
}

func (f *Flow) callbackReturnURL(ctx context.Context, endpointID int64, result string) string {
	base := strings.TrimRight(strings.TrimSpace(f.returnBaseURLEffective(ctx)), "/")
	if endpointID > 0 {
		return fmt.Sprintf("%s/admin/endpoints/%d/codex-accounts?oauth=%s", base, endpointID, result)
	}
	return fmt.Sprintf("%s/admin/channels", base)
}

func (f *Flow) returnBaseURLEffective(ctx context.Context) string {
	if ctx != nil && f.st != nil {
		if v, ok, err := f.st.GetStringAppSetting(ctx, store.SettingSiteBaseURL); err == nil && ok {
			if normalized, err := config.NormalizeHTTPBaseURL(v, "site_base_url"); err == nil && normalized != "" {
				return normalized
			}
		}
	}
	return f.returnBaseURL
}

func (f *Flow) handleCallback(w http.ResponseWriter, r *http.Request) {
	if errText := strings.TrimSpace(r.URL.Query().Get("error")); errText != "" {
		errDesc := strings.TrimSpace(r.URL.Query().Get("error_description"))
		msg := "上游返回错误: " + errText
		if errDesc != "" {
			msg += " - " + errDesc
		}
		writeCallbackHTML(w, http.StatusBadRequest, "Codex OAuth 失败", msg, f.callbackReturnURL(r.Context(), 0, "error"))
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || state == "" {
		writeCallbackHTML(w, http.StatusBadRequest, "Codex OAuth 失败", UserMessage(Wrap(ErrCallbackInvalidParams, fmt.Errorf("missing code/state"))), f.callbackReturnURL(r.Context(), 0, "error"))
		return
	}

	pending, ok, err := f.getAndDeletePending(r.Context(), state)
	if err != nil {
		writeCallbackHTML(w, http.StatusBadGateway, "Codex OAuth 失败", UserMessage(Wrap(ErrStoreFailed, err)), f.callbackReturnURL(r.Context(), 0, "error"))
		return
	}
	if !ok {
		writeCallbackHTML(w, http.StatusBadRequest, "Codex OAuth 失败", UserMessage(Wrap(ErrCallbackInvalidOrExpiredState, fmt.Errorf("invalid state"))), f.callbackReturnURL(r.Context(), 0, "error"))
		return
	}
	if time.Since(pending.CreatedAt) > f.ttl {
		writeCallbackHTML(w, http.StatusBadRequest, "Codex OAuth 失败", UserMessage(Wrap(ErrCallbackExpiredState, fmt.Errorf("expired state"))), f.callbackReturnURL(r.Context(), pending.EndpointID, "error"))
		return
	}

	if f.cookieStore == nil {
		writeCallbackHTML(w, http.StatusUnauthorized, "Codex OAuth 失败", UserMessage(Wrap(ErrCallbackMissingSessionCookie, fmt.Errorf("missing cookie store"))), f.callbackReturnURL(r.Context(), pending.EndpointID, "error"))
		return
	}
	cookieSession, err := f.cookieStore.Get(r, f.cookieName)
	if err != nil || cookieSession == nil {
		writeCallbackHTML(w, http.StatusUnauthorized, "Codex OAuth 失败", UserMessage(Wrap(ErrCallbackMissingSessionCookie, fmt.Errorf("missing session cookie"))), f.callbackReturnURL(r.Context(), pending.EndpointID, "error"))
		return
	}
	userID, ok := int64FromSessionValue(cookieSession.Values["id"])
	if !ok || userID <= 0 {
		writeCallbackHTML(w, http.StatusUnauthorized, "Codex OAuth 失败", UserMessage(Wrap(ErrCallbackInvalidSession, fmt.Errorf("missing id"))), f.callbackReturnURL(r.Context(), pending.EndpointID, "error"))
		return
	}
	if pending.ActorUserID > 0 && userID != pending.ActorUserID {
		writeCallbackHTML(w, http.StatusForbidden, "Codex OAuth 失败", UserMessage(Wrap(ErrCallbackActorMismatch, fmt.Errorf("actor mismatch"))), f.callbackReturnURL(r.Context(), pending.EndpointID, "error"))
		return
	}
	user, err := f.st.GetUserByID(r.Context(), userID)
	if err != nil {
		writeCallbackHTML(w, http.StatusUnauthorized, "Codex OAuth 失败", UserMessage(Wrap(ErrCallbackUserNotFound, err)), f.callbackReturnURL(r.Context(), pending.EndpointID, "error"))
		return
	}
	if user.Role != store.UserRoleRoot {
		writeCallbackHTML(w, http.StatusForbidden, "Codex OAuth 失败", UserMessage(Wrap(ErrCallbackForbidden, fmt.Errorf("insufficient role"))), f.callbackReturnURL(r.Context(), pending.EndpointID, "error"))
		return
	}

	res, err := f.client.ExchangeCode(r.Context(), code, pending.CodeVerifier)
	if err != nil {
		writeCallbackHTML(w, http.StatusBadGateway, "Codex OAuth 失败", UserMessage(Wrap(ErrCodeExchangeFailed, err)), f.callbackReturnURL(r.Context(), pending.EndpointID, "error"))
		return
	}

	claims, err := ParseIDTokenClaims(res.IDToken)
	if err != nil {
		writeCallbackHTML(w, http.StatusBadGateway, "Codex OAuth 失败", UserMessage(Wrap(ErrIDTokenParseFailed, err)), f.callbackReturnURL(r.Context(), pending.EndpointID, "error"))
		return
	}
	accountID := strings.TrimSpace(claims.AccountID)
	if accountID == "" {
		writeCallbackHTML(w, http.StatusBadGateway, "Codex OAuth 失败", UserMessage(Wrap(ErrMissingAccountID, fmt.Errorf("missing account_id"))), f.callbackReturnURL(r.Context(), pending.EndpointID, "error"))
		return
	}
	email := strings.TrimSpace(claims.Email)
	var emailPtr *string
	if email != "" {
		emailPtr = &email
	}
	idToken := res.IDToken
	idTokenPtr := &idToken
	if idToken == "" {
		idTokenPtr = nil
	}

	if _, err := f.st.CreateCodexOAuthAccount(r.Context(), pending.EndpointID, accountID, emailPtr, res.AccessToken, res.RefreshToken, idTokenPtr, res.ExpiresAt); err != nil {
		writeCallbackHTML(w, http.StatusInternalServerError, "Codex OAuth 失败", UserMessage(Wrap(ErrStoreFailed, err)), f.callbackReturnURL(r.Context(), pending.EndpointID, "error"))
		return
	}

	writeCallbackHTML(w, http.StatusOK, "Codex OAuth 授权成功", "已成功保存账号凭据，正在跳转到管理后台。", f.callbackReturnURL(r.Context(), pending.EndpointID, "ok"))
}

func int64FromSessionValue(v any) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case int:
		return int64(x), true
	case float64:
		return int64(x), true
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func (f *Flow) Complete(ctx context.Context, endpointID int64, actorUserID int64, state string, code string) error {
	code = strings.TrimSpace(code)
	state = strings.TrimSpace(state)
	if code == "" || state == "" {
		return Wrap(ErrCallbackInvalidParams, fmt.Errorf("missing code/state"))
	}

	pending, ok, err := f.getPending(ctx, state)
	if err != nil {
		return Wrap(ErrStoreFailed, err)
	}
	if !ok {
		return Wrap(ErrCallbackInvalidOrExpiredState, fmt.Errorf("invalid state"))
	}
	if time.Since(pending.CreatedAt) > f.ttl {
		_ = f.deletePending(ctx, state)
		return Wrap(ErrCallbackExpiredState, fmt.Errorf("expired state"))
	}
	if pending.EndpointID != endpointID {
		return Wrap(ErrCallbackInvalidParams, fmt.Errorf("endpoint mismatch"))
	}
	if pending.ActorUserID != actorUserID {
		return Wrap(ErrCallbackActorMismatch, fmt.Errorf("actor mismatch"))
	}

	if err := f.deletePending(ctx, state); err != nil {
		return Wrap(ErrStoreFailed, err)
	}

	res, err := f.client.ExchangeCode(ctx, code, pending.CodeVerifier)
	if err != nil {
		return Wrap(ErrCodeExchangeFailed, err)
	}
	claims, err := ParseIDTokenClaims(res.IDToken)
	if err != nil {
		return Wrap(ErrIDTokenParseFailed, err)
	}
	accountID := strings.TrimSpace(claims.AccountID)
	if accountID == "" {
		return Wrap(ErrMissingAccountID, fmt.Errorf("missing account_id"))
	}
	email := strings.TrimSpace(claims.Email)
	var emailPtr *string
	if email != "" {
		emailPtr = &email
	}

	idToken := strings.TrimSpace(res.IDToken)
	var idTokenPtr *string
	if idToken != "" {
		idTokenPtr = &idToken
	}

	if _, err := f.st.CreateCodexOAuthAccount(ctx, pending.EndpointID, accountID, emailPtr, res.AccessToken, res.RefreshToken, idTokenPtr, res.ExpiresAt); err != nil {
		return Wrap(ErrStoreFailed, err)
	}
	return nil
}

func (f *Flow) pruneExpiredPending(ctx context.Context, now time.Time) error {
	if f.st != nil {
		return f.st.DeleteCodexOAuthPendingBefore(ctx, now.Add(-f.ttl))
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	for k, p := range f.pending {
		if now.Sub(p.CreatedAt) > f.ttl {
			delete(f.pending, k)
		}
	}
	return nil
}

func (f *Flow) putPending(ctx context.Context, state string, p Pending) error {
	if f.st != nil {
		return f.st.CreateCodexOAuthPending(ctx, state, p.EndpointID, p.ActorUserID, p.CodeVerifier, p.CreatedAt)
	}

	f.mu.Lock()
	f.pending[state] = p
	f.mu.Unlock()
	return nil
}

func (f *Flow) getPending(ctx context.Context, state string) (Pending, bool, error) {
	if f.st != nil {
		p, ok, err := f.st.GetCodexOAuthPending(ctx, state)
		if err != nil {
			return Pending{}, false, err
		}
		if !ok {
			return Pending{}, false, nil
		}
		return Pending{
			EndpointID:   p.EndpointID,
			ActorUserID:  p.ActorUserID,
			CodeVerifier: p.CodeVerifier,
			CreatedAt:    p.CreatedAt,
		}, true, nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.pending[state]
	return p, ok, nil
}

func (f *Flow) deletePending(ctx context.Context, state string) error {
	if f.st != nil {
		return f.st.DeleteCodexOAuthPending(ctx, state)
	}

	f.mu.Lock()
	delete(f.pending, state)
	f.mu.Unlock()
	return nil
}

func (f *Flow) getAndDeletePending(ctx context.Context, state string) (Pending, bool, error) {
	p, ok, err := f.getPending(ctx, state)
	if err != nil || !ok {
		return Pending{}, ok, err
	}
	if err := f.deletePending(ctx, state); err != nil {
		return Pending{}, false, err
	}
	return p, true, nil
}
