package router

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	"realms/internal/auth"
	"realms/internal/scheduler"
	"realms/internal/store"
)

func TestPinnedChannelInfo_ReadsGlobalPointerFromAppSettings(t *testing.T) {
	gin.SetMode(gin.TestMode)

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
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	userID, err := st.CreateUser(ctx, "root@example.com", "root", pwHash, store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	chID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "channel-1", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}

	raw, err := store.SchedulerChannelPointerState{
		V:             1,
		ChannelID:     chID,
		Pinned:        false,
		MovedAtUnixMS: time.Now().UnixMilli(),
		Reason:        "route",
	}.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := st.UpsertAppSetting(ctx, store.SettingSchedulerChannelPointer, raw); err != nil {
		t.Fatalf("UpsertAppSetting: %v", err)
	}

	engine := gin.New()
	engine.Use(gin.Recovery())
	cookieName := "realms_session"
	sessionStore := cookie.NewStore([]byte("test-secret"))
	sessionStore.Options(sessions.Options{
		Path:     "/",
		MaxAge:   2592000,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	engine.Use(sessions.Sessions(cookieName, sessionStore))

	sched := scheduler.New(st)
	sched.SetPointerStore(st)
	_ = sched.SyncChannelPointerFromStore(context.Background())

	SetRouter(engine, Options{
		Store:             st,
		Sched:             sched,
		SelfMode:          false,
		FrontendIndexPage: []byte("<!doctype html><html><body>INDEX</body></html>"),
	})

	// login
	loginBody, _ := json.Marshal(map[string]any{
		"login":    "root@example.com",
		"password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/user/login", bytes.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", rr.Code, rr.Body.String())
	}
	sessionCookie := ""
	for _, c := range rr.Result().Cookies() {
		if c.Name == cookieName {
			sessionCookie = c.String()
			break
		}
	}
	if sessionCookie == "" {
		t.Fatalf("expected session cookie %q", cookieName)
	}

	// pinned
	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/channel/pinned", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("pinned status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Available       bool   `json:"available"`
			PinnedActive    bool   `json:"pinned_active"`
			PinnedChannelID int64  `json:"pinned_channel_id"`
			PinnedChannel   string `json:"pinned_channel"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got message=%q", resp.Message)
	}
	if !resp.Data.Available {
		t.Fatalf("expected available=true")
	}
	if !resp.Data.PinnedActive || resp.Data.PinnedChannelID != chID {
		t.Fatalf("expected pinned_active=true pinned_channel_id=%d, got active=%v id=%d", chID, resp.Data.PinnedActive, resp.Data.PinnedChannelID)
	}
	if resp.Data.PinnedChannel != "channel-1" {
		t.Fatalf("unexpected pinned_channel=%q", resp.Data.PinnedChannel)
	}
}
