package admin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"realms/internal/admin"
	"realms/internal/auth"
	"realms/internal/config"
	"realms/internal/store"
)

func newTestAdminServer(t *testing.T) (*admin.Server, *store.Store, context.Context, int64) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "realms.db") + "?_busy_timeout=1000"

	db, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.EnsureSQLiteSchema(db); err != nil {
		t.Fatalf("EnsureSQLiteSchema: %v", err)
	}

	st := store.New(db)
	st.SetDialect(store.DialectSQLite)

	ctx := context.Background()
	userID, err := st.CreateUser(ctx, "root@example.com", "root", []byte("x"), store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	srv, err := admin.NewServer(
		st,
		nil,
		nil,
		false,
		false,
		config.SMTPConfig{},
		config.BillingConfig{},
		config.PaymentConfig{},
		"http://example.com",
		"UTC",
		false,
		nil,
		config.TicketsConfig{},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	return srv, st, ctx, userID
}

func withPrincipal(req *http.Request, userID int64) *http.Request {
	csrf := "csrf"
	return req.WithContext(auth.WithPrincipal(req.Context(), auth.Principal{
		ActorType: auth.ActorTypeSession,
		UserID:    userID,
		CSRFToken: &csrf,
	}))
}

func TestEndpoints_RedirectsToChannelsModal(t *testing.T) {
	srv, st, ctx, userID := newTestAdminServer(t)

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ch1", store.DefaultGroupName, 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}

	req := httptest.NewRequest("GET", "/admin/channels/"+strconv.FormatInt(channelID, 10)+"/endpoints?msg=ok", nil)
	req.SetPathValue("channel_id", strconv.FormatInt(channelID, 10))
	req = withPrincipal(req, userID)

	rr := httptest.NewRecorder()
	srv.Endpoints(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status: got %d want %d", rr.Code, http.StatusFound)
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "/admin/channels?") {
		t.Fatalf("Location: got %q want prefix %q", loc, "/admin/channels?")
	}
	if !strings.Contains(loc, "open_channel_settings="+url.QueryEscape(strconv.FormatInt(channelID, 10))) {
		t.Fatalf("Location: expected open_channel_settings, got %q", loc)
	}
	if !strings.Contains(loc, "msg=ok") {
		t.Fatalf("Location: expected msg=ok, got %q", loc)
	}
	if !strings.HasSuffix(loc, "#keys") {
		t.Fatalf("Location: expected suffix #keys, got %q", loc)
	}
}

func TestEndpoints_CodexOAuthRedirectsToAccounts(t *testing.T) {
	srv, st, ctx, userID := newTestAdminServer(t)

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeCodexOAuth, "codex", store.DefaultGroupName, 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}

	req := httptest.NewRequest("GET", "/admin/channels/"+strconv.FormatInt(channelID, 10)+"/endpoints", nil)
	req.SetPathValue("channel_id", strconv.FormatInt(channelID, 10))
	req = withPrincipal(req, userID)

	rr := httptest.NewRecorder()
	srv.Endpoints(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status: got %d want %d", rr.Code, http.StatusFound)
	}
	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "open_channel_settings="+url.QueryEscape(strconv.FormatInt(channelID, 10))) {
		t.Fatalf("Location: expected open_channel_settings, got %q", loc)
	}
	if !strings.HasSuffix(loc, "#accounts") {
		t.Fatalf("Location: expected suffix #accounts, got %q", loc)
	}
}

func TestChannelModels_RedirectsToChannelsModal(t *testing.T) {
	srv, st, ctx, userID := newTestAdminServer(t)

	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ch1", store.DefaultGroupName, 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}

	req := httptest.NewRequest("GET", "/admin/channels/"+strconv.FormatInt(channelID, 10)+"/models?err=bad", nil)
	req.SetPathValue("channel_id", strconv.FormatInt(channelID, 10))
	req = withPrincipal(req, userID)

	rr := httptest.NewRecorder()
	srv.ChannelModels(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status: got %d want %d", rr.Code, http.StatusFound)
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "/admin/channels?") {
		t.Fatalf("Location: got %q want prefix %q", loc, "/admin/channels?")
	}
	if !strings.Contains(loc, "open_channel_settings="+url.QueryEscape(strconv.FormatInt(channelID, 10))) {
		t.Fatalf("Location: expected open_channel_settings, got %q", loc)
	}
	if !strings.Contains(loc, "err=bad") {
		t.Fatalf("Location: expected err=bad, got %q", loc)
	}
	if !strings.HasSuffix(loc, "#models") {
		t.Fatalf("Location: expected suffix #models, got %q", loc)
	}
}
