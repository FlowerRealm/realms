package admin_test

import (
	"context"
	"encoding/json"
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

func TestCreateEndpoint_AjaxOK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "realms.db") + "?_busy_timeout=1000"

	db, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer db.Close()
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
	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "ch1", store.DefaultGroupName, 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
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

	csrf := "csrf"
	form := url.Values{}
	form.Set("base_url", "https://api.openai.com")
	req := httptest.NewRequest("POST", "/admin/channels/"+strconv.FormatInt(channelID, 10)+"/endpoints", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	req.Header.Set("X-Realms-Ajax", "1")
	req.SetPathValue("channel_id", strconv.FormatInt(channelID, 10))
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.Principal{
		ActorType: auth.ActorTypeSession,
		UserID:    userID,
		CSRFToken: &csrf,
	}))

	rr := httptest.NewRecorder()
	srv.CreateEndpoint(rr, req)

	if rr.Code != 200 {
		t.Fatalf("status: got %d want %d, body=%s", rr.Code, 200, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(strings.ToLower(ct), "application/json") {
		t.Fatalf("Content-Type: got %q", ct)
	}

	var resp struct {
		OK     bool   `json:"ok"`
		Notice string `json:"notice"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true, got ok=false err=%q notice=%q", resp.Error, resp.Notice)
	}
	if strings.TrimSpace(resp.Notice) == "" {
		t.Fatalf("expected notice, got empty")
	}

	ep, err := st.GetUpstreamEndpointByChannelID(ctx, channelID)
	if err != nil {
		t.Fatalf("GetUpstreamEndpointByChannelID: %v", err)
	}
	if ep.BaseURL != "https://api.openai.com" {
		t.Fatalf("base_url: got %q want %q", ep.BaseURL, "https://api.openai.com")
	}
}
