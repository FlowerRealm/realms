package codexoauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"realms/internal/config"
)

type roundTripperFunc func(r *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestNewPKCE(t *testing.T) {
	verifier, challenge, err := NewPKCE()
	if err != nil {
		t.Fatalf("NewPKCE: %v", err)
	}
	if verifier == "" || challenge == "" {
		t.Fatalf("NewPKCE returned empty verifier/challenge")
	}
	sum := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if challenge != want {
		t.Fatalf("challenge mismatch: got %q want %q", challenge, want)
	}
}

func TestClientBuildAuthorizeURL(t *testing.T) {
	c := NewClient(config.CodexOAuthConfig{
		AuthorizeURL: "https://auth.openai.com/oauth/authorize",
		ClientID:     "app_test",
		RedirectURI:  "http://localhost:1455/auth/callback",
		Scope:        "openid email profile offline_access",
		Prompt:       "login",
	})
	got, err := c.BuildAuthorizeURL("state123", "challenge123")
	if err != nil {
		t.Fatalf("BuildAuthorizeURL: %v", err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	q := u.Query()
	check := func(k, want string) {
		if q.Get(k) != want {
			t.Fatalf("query[%s] = %q, want %q", k, q.Get(k), want)
		}
	}
	check("response_type", "code")
	check("client_id", "app_test")
	check("redirect_uri", "http://localhost:1455/auth/callback")
	check("scope", "openid email profile offline_access")
	check("state", "state123")
	check("code_challenge", "challenge123")
	check("code_challenge_method", "S256")
	check("prompt", "login")
	check("codex_cli_simplified_flow", "true")
	check("id_token_add_organizations", "true")
}

func TestParseIDTokenClaims(t *testing.T) {
	t.Run("nested_openai_auth", func(t *testing.T) {
		payload := `{"email":"user@example.com","https://api.openai.com/auth":{"chatgpt_account_id":"user-123","chatgpt_plan_type":"plus","chatgpt_subscription_active_start":1700000000,"chatgpt_subscription_active_until":1700000100}}`
		token := "e30." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".sig"

		claims, err := ParseIDTokenClaims(token)
		if err != nil {
			t.Fatalf("ParseIDTokenClaims: %v", err)
		}
		if claims.AccountID != "user-123" {
			t.Fatalf("AccountID = %q, want %q", claims.AccountID, "user-123")
		}
		if claims.Email != "user@example.com" {
			t.Fatalf("Email = %q, want %q", claims.Email, "user@example.com")
		}
		if claims.PlanType != "plus" {
			t.Fatalf("PlanType = %q, want %q", claims.PlanType, "plus")
		}
		if got, ok := claims.SubscriptionActiveStart.(float64); !ok || got != 1700000000 {
			t.Fatalf("SubscriptionActiveStart = (%T)%v, want float64(%v)", claims.SubscriptionActiveStart, claims.SubscriptionActiveStart, 1700000000)
		}
		if got, ok := claims.SubscriptionActiveUntil.(float64); !ok || got != 1700000100 {
			t.Fatalf("SubscriptionActiveUntil = (%T)%v, want float64(%v)", claims.SubscriptionActiveUntil, claims.SubscriptionActiveUntil, 1700000100)
		}
	})

	t.Run("fallback_top_level", func(t *testing.T) {
		payload := `{"email":"u@x.com","chatgpt_account_id":"user-abc","plan_type":"pro"}`
		token := "e30." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".sig"

		claims, err := ParseIDTokenClaims(token)
		if err != nil {
			t.Fatalf("ParseIDTokenClaims: %v", err)
		}
		if claims.AccountID != "user-abc" {
			t.Fatalf("AccountID = %q, want %q", claims.AccountID, "user-abc")
		}
		if claims.Email != "u@x.com" {
			t.Fatalf("Email = %q, want %q", claims.Email, "u@x.com")
		}
		if claims.PlanType != "pro" {
			t.Fatalf("PlanType = %q, want %q", claims.PlanType, "pro")
		}
	})
}

func TestParseOAuthCallback(t *testing.T) {
	t.Run("full_url", func(t *testing.T) {
		cb, err := ParseOAuthCallback("http://localhost:1455/auth/callback?code=c&state=s")
		if err != nil {
			t.Fatalf("ParseOAuthCallback: %v", err)
		}
		if cb == nil || cb.Code != "c" || cb.State != "s" {
			t.Fatalf("unexpected callback: %#v", cb)
		}
	})

	t.Run("query_only", func(t *testing.T) {
		cb, err := ParseOAuthCallback("code=c&state=s")
		if err != nil {
			t.Fatalf("ParseOAuthCallback: %v", err)
		}
		if cb == nil || cb.Code != "c" || cb.State != "s" {
			t.Fatalf("unexpected callback: %#v", cb)
		}
	})

	t.Run("error", func(t *testing.T) {
		cb, err := ParseOAuthCallback("http://localhost:1455/auth/callback?error=access_denied&error_description=cancelled")
		if err != nil {
			t.Fatalf("ParseOAuthCallback: %v", err)
		}
		if cb == nil || cb.Error != "access_denied" || cb.ErrorDescription != "cancelled" {
			t.Fatalf("unexpected callback: %#v", cb)
		}
	})
}

func TestFlowPendingCleanupAndOneTimeState(t *testing.T) {
	c := NewClient(config.CodexOAuthConfig{
		AuthorizeURL: "https://auth.openai.com/oauth/authorize",
		ClientID:     "app_test",
		RedirectURI:  "http://localhost:1455/auth/callback",
		Scope:        "openid email profile offline_access",
	})
	f := NewFlow(nil, c, "sid", "http://localhost:8080")
	f.ttl = 1 * time.Minute
	f.pending["old"] = Pending{CreatedAt: time.Now().Add(-2 * f.ttl)}

	_, err := f.Start(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, ok := f.pending["old"]; ok {
		t.Fatalf("expected expired pending state to be pruned")
	}

	f.pending["once"] = Pending{CreatedAt: time.Now()}
	if _, ok, err := f.getAndDeletePending(context.Background(), "once"); err != nil || !ok {
		t.Fatalf("expected first getAndDeletePending to succeed")
	}
	if _, ok, err := f.getAndDeletePending(context.Background(), "once"); err != nil {
		t.Fatalf("expected second getAndDeletePending to succeed without error: %v", err)
	} else if ok {
		t.Fatalf("expected second getAndDeletePending to fail")
	}
}

func TestClientRefreshRetryAndInvalidGrant(t *testing.T) {
	t.Run("retry_on_5xx", func(t *testing.T) {
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if calls == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("oops"))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"access_token":"at","expires_in":60}`))
		}))
		defer srv.Close()

		c := NewClient(config.CodexOAuthConfig{
			TokenURL:    srv.URL,
			ClientID:    "app_test",
			Scope:       "openid",
			Prompt:      "login",
			RedirectURI: "http://localhost:1455/auth/callback",
		})
		c.refreshBackoffs = []time.Duration{0}

		res, err := c.Refresh(context.Background(), "rt")
		if err != nil {
			t.Fatalf("Refresh: %v", err)
		}
		if res.AccessToken != "at" {
			t.Fatalf("AccessToken = %q, want %q", res.AccessToken, "at")
		}
		if calls != 2 {
			t.Fatalf("calls = %d, want 2", calls)
		}
	})

	t.Run("no_retry_on_invalid_grant", func(t *testing.T) {
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"expired"}`))
		}))
		defer srv.Close()

		c := NewClient(config.CodexOAuthConfig{
			TokenURL: srv.URL,
			ClientID: "app_test",
		})
		c.refreshBackoffs = []time.Duration{0, 0, 0}

		_, err := c.Refresh(context.Background(), "rt")
		if err == nil {
			t.Fatalf("expected error")
		}
		if code, ok := CodeOf(err); !ok || code != ErrRefreshFailed {
			t.Fatalf("CodeOf(err) = (%v, %v), want (%v, true)", code, ok, ErrRefreshFailed)
		}
		var te *TokenEndpointError
		if !errors.As(err, &te) {
			t.Fatalf("expected TokenEndpointError")
		}
		if te.StatusCode != http.StatusBadRequest || te.ErrorCode != "invalid_grant" {
			t.Fatalf("TokenEndpointError = (%d, %q), want (%d, %q)", te.StatusCode, te.ErrorCode, http.StatusBadRequest, "invalid_grant")
		}
		if calls != 1 {
			t.Fatalf("calls = %d, want 1", calls)
		}
	})
}

func TestClientFetchQuota(t *testing.T) {
	t.Run("codex_api_style", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Fatalf("method = %s, want GET", r.Method)
			}
			if r.URL.Path != "/api/codex/usage" {
				t.Fatalf("path = %q, want %q", r.URL.Path, "/api/codex/usage")
			}
			if got := r.Header.Get("Authorization"); got != "Bearer at" {
				t.Fatalf("Authorization = %q, want %q", got, "Bearer at")
			}
			if got := r.Header.Get("Chatgpt-Account-Id"); got != "user-123" {
				t.Fatalf("Chatgpt-Account-Id = %q, want %q", got, "user-123")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"plan_type":"pro","rate_limit":{"primary_window":{"used_percent":42,"reset_at":1700000000},"secondary_window":{"used_percent":5,"reset_at":1700000100}},"credits":{"has_credits":true,"unlimited":false,"balance":"17.5"}}`))
		}))
		defer srv.Close()

		c := NewClient(config.CodexOAuthConfig{TokenURL: "http://example.com", ClientID: "app_test"})
		out, err := c.FetchQuota(context.Background(), srv.URL, "at", "user-123")
		if err != nil {
			t.Fatalf("FetchQuota: %v", err)
		}
		if out.PlanType != "pro" {
			t.Fatalf("PlanType = %q, want %q", out.PlanType, "pro")
		}
		if out.CreditsHasCredits == nil || *out.CreditsHasCredits != true {
			t.Fatalf("CreditsHasCredits = %#v, want true", out.CreditsHasCredits)
		}
		if out.CreditsUnlimited == nil || *out.CreditsUnlimited != false {
			t.Fatalf("CreditsUnlimited = %#v, want false", out.CreditsUnlimited)
		}
		if out.CreditsBalance == nil || *out.CreditsBalance != "17.5" {
			t.Fatalf("CreditsBalance = %#v, want %q", out.CreditsBalance, "17.5")
		}
		if out.PrimaryUsedPercent == nil || *out.PrimaryUsedPercent != 42 {
			t.Fatalf("PrimaryUsedPercent = %#v, want %d", out.PrimaryUsedPercent, 42)
		}
		if out.PrimaryResetAt == nil || out.PrimaryResetAt.UTC().Format(time.RFC3339) != time.Unix(1700000000, 0).UTC().Format(time.RFC3339) {
			t.Fatalf("PrimaryResetAt = %#v, want %s", out.PrimaryResetAt, time.Unix(1700000000, 0).UTC().Format(time.RFC3339))
		}
		if out.SecondaryUsedPercent == nil || *out.SecondaryUsedPercent != 5 {
			t.Fatalf("SecondaryUsedPercent = %#v, want %d", out.SecondaryUsedPercent, 5)
		}
		if out.SecondaryResetAt == nil || out.SecondaryResetAt.UTC().Format(time.RFC3339) != time.Unix(1700000100, 0).UTC().Format(time.RFC3339) {
			t.Fatalf("SecondaryResetAt = %#v, want %s", out.SecondaryResetAt, time.Unix(1700000100, 0).UTC().Format(time.RFC3339))
		}
	})

	t.Run("chatgpt_backend_style", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/backend-api/wham/usage" {
				t.Fatalf("path = %q, want %q", r.URL.Path, "/backend-api/wham/usage")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"plan_type":"plus"}`))
		}))
		defer srv.Close()

		c := NewClient(config.CodexOAuthConfig{TokenURL: "http://example.com", ClientID: "app_test"})
		out, err := c.FetchQuota(context.Background(), srv.URL+"/backend-api/codex", "at", "")
		if err != nil {
			t.Fatalf("FetchQuota: %v", err)
		}
		if out.PlanType != "plus" {
			t.Fatalf("PlanType = %q, want %q", out.PlanType, "plus")
		}
	})

	t.Run("non_2xx", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("unauthorized"))
		}))
		defer srv.Close()

		c := NewClient(config.CodexOAuthConfig{TokenURL: "http://example.com", ClientID: "app_test"})
		_, err := c.FetchQuota(context.Background(), srv.URL, "at", "user-123")
		if err == nil {
			t.Fatalf("expected error")
		}
		var se *HTTPStatusError
		if !errors.As(err, &se) {
			t.Fatalf("expected HTTPStatusError, got %T", err)
		}
		if se.StatusCode != http.StatusUnauthorized {
			t.Fatalf("StatusCode = %d, want %d", se.StatusCode, http.StatusUnauthorized)
		}
	})

	t.Run("retry_on_eof", func(t *testing.T) {
		calls := 0
		c := NewClient(config.CodexOAuthConfig{TokenURL: "http://example.com", ClientID: "app_test"})
		c.http.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Method != http.MethodGet {
				t.Fatalf("method = %s, want GET", r.Method)
			}
			if r.URL.Path != "/api/codex/usage" {
				t.Fatalf("path = %q, want %q", r.URL.Path, "/api/codex/usage")
			}
			if got := r.Header.Get("Authorization"); got != "Bearer at" {
				t.Fatalf("Authorization = %q, want %q", got, "Bearer at")
			}
			if got := r.Header.Get("Accept"); got != "application/json" {
				t.Fatalf("Accept = %q, want %q", got, "application/json")
			}
			if got := r.Header.Get("Chatgpt-Account-Id"); got != "user-123" {
				t.Fatalf("Chatgpt-Account-Id = %q, want %q", got, "user-123")
			}
			if calls == 1 {
				return nil, io.EOF
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"plan_type":"plus"}`)),
			}, nil
		})

		out, err := c.FetchQuota(context.Background(), "https://example.com", "at", "user-123")
		if err != nil {
			t.Fatalf("FetchQuota: %v", err)
		}
		if calls != 2 {
			t.Fatalf("calls = %d, want 2", calls)
		}
		if out.PlanType != "plus" {
			t.Fatalf("PlanType = %q, want %q", out.PlanType, "plus")
		}
	})

	t.Run("fallback_backend_api", func(t *testing.T) {
		calls := 0
		c := NewClient(config.CodexOAuthConfig{TokenURL: "http://example.com", ClientID: "app_test"})
		c.http.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			switch calls {
			case 1, 2:
				if r.URL.Path != "/backend-api/wham/usage" {
					t.Fatalf("path = %q, want %q", r.URL.Path, "/backend-api/wham/usage")
				}
				return nil, io.EOF
			case 3:
				if r.URL.Path != "/backend-api/codex/usage" {
					t.Fatalf("path = %q, want %q", r.URL.Path, "/backend-api/codex/usage")
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"plan_type":"plus"}`)),
				}, nil
			default:
				t.Fatalf("unexpected call %d, path=%q", calls, r.URL.Path)
				return nil, nil
			}
		})

		out, err := c.FetchQuota(context.Background(), "https://example.com/backend-api/codex", "at", "user-123")
		if err != nil {
			t.Fatalf("FetchQuota: %v", err)
		}
		if calls != 3 {
			t.Fatalf("calls = %d, want 3", calls)
		}
		if out.PlanType != "plus" {
			t.Fatalf("PlanType = %q, want %q", out.PlanType, "plus")
		}
	})
}
