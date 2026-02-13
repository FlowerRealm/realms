package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	"realms/internal/auth"
	"realms/internal/store"
)

func setupRootSession(t *testing.T, st *store.Store) (*gin.Engine, string, int64) {
	t.Helper()

	ctx := context.Background()
	pwHash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	userID, err := st.CreateUser(ctx, "root@example.com", "root", pwHash, store.UserRoleRoot)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
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

	SetRouter(engine, Options{
		Store:             st,
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

	return engine, sessionCookie, userID
}

func TestAdminChannelGroupPointer_Get_Default(t *testing.T) {
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

	engine, sessionCookie, userID := setupRootSession(t, st)

	ctx := context.Background()
	groupID, err := st.CreateChannelGroup(ctx, "g1", nil, 1, store.DefaultGroupPriceMultiplier, 5)
	if err != nil {
		t.Fatalf("CreateChannelGroup: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://example.com/api/admin/channel-groups/%d/pointer", groupID), nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get pointer status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			GroupID   int64 `json:"group_id"`
			ChannelID int64 `json:"channel_id"`
			Pinned    bool  `json:"pinned"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got message=%q", resp.Message)
	}
	if resp.Data.GroupID != groupID || resp.Data.ChannelID != 0 || resp.Data.Pinned {
		t.Fatalf("unexpected resp.data: %+v", resp.Data)
	}
}

func TestAdminChannelGroupPointer_PutAndGet_RoundTrip(t *testing.T) {
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

	engine, sessionCookie, userID := setupRootSession(t, st)

	ctx := context.Background()
	parentID, err := st.CreateChannelGroup(ctx, "g1", nil, 1, store.DefaultGroupPriceMultiplier, 5)
	if err != nil {
		t.Fatalf("CreateChannelGroup parent: %v", err)
	}
	childID, err := st.CreateChannelGroup(ctx, "g1_child", nil, 1, store.DefaultGroupPriceMultiplier, 5)
	if err != nil {
		t.Fatalf("CreateChannelGroup child: %v", err)
	}
	if err := st.AddChannelGroupMemberGroup(ctx, parentID, childID, 0, false); err != nil {
		t.Fatalf("AddChannelGroupMemberGroup: %v", err)
	}
	channelID, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "c1", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel: %v", err)
	}
	if err := st.AddChannelGroupMemberChannel(ctx, childID, channelID, 0, false); err != nil {
		t.Fatalf("AddChannelGroupMemberChannel: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"channel_id": channelID,
	})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("http://example.com/api/admin/channel-groups/%d/pointer", parentID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("put pointer status=%d body=%s", rr.Code, rr.Body.String())
	}
	var putResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &putResp); err != nil {
		t.Fatalf("json.Unmarshal put: %v", err)
	}
	if !putResp.Success {
		t.Fatalf("expected put success, got message=%q", putResp.Message)
	}

	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://example.com/api/admin/channel-groups/%d/pointer", parentID), nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get pointer status=%d body=%s", rr.Code, rr.Body.String())
	}
	var getResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			GroupID     int64  `json:"group_id"`
			ChannelID   int64  `json:"channel_id"`
			ChannelName string `json:"channel_name"`
			Pinned      bool   `json:"pinned"`
			MovedAt     string `json:"moved_at"`
			Reason      string `json:"reason"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("json.Unmarshal get: %v", err)
	}
	if !getResp.Success {
		t.Fatalf("expected get success, got message=%q", getResp.Message)
	}
	if getResp.Data.GroupID != parentID || getResp.Data.ChannelID != channelID || !getResp.Data.Pinned {
		t.Fatalf("unexpected data: %+v", getResp.Data)
	}
	if getResp.Data.ChannelName != "c1" {
		t.Fatalf("expected channel_name=c1, got %q", getResp.Data.ChannelName)
	}
	if getResp.Data.MovedAt == "" || getResp.Data.Reason == "" {
		t.Fatalf("expected moved_at/reason to be non-empty, got moved_at=%q reason=%q", getResp.Data.MovedAt, getResp.Data.Reason)
	}
}

func TestAdminChannelGroupPointer_Put_RejectChannelNotInGroup(t *testing.T) {
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

	engine, sessionCookie, userID := setupRootSession(t, st)

	ctx := context.Background()
	groupID, err := st.CreateChannelGroup(ctx, "g1", nil, 1, store.DefaultGroupPriceMultiplier, 5)
	if err != nil {
		t.Fatalf("CreateChannelGroup: %v", err)
	}
	ch1, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "c1", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel c1: %v", err)
	}
	if err := st.AddChannelGroupMemberChannel(ctx, groupID, ch1, 0, false); err != nil {
		t.Fatalf("AddChannelGroupMemberChannel: %v", err)
	}
	ch2, err := st.CreateUpstreamChannel(ctx, store.UpstreamTypeOpenAICompatible, "c2", "", 0, false, false, false, false)
	if err != nil {
		t.Fatalf("CreateUpstreamChannel c2: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"channel_id": ch2,
		"pinned":     true,
	})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("http://example.com/api/admin/channel-groups/%d/pointer", groupID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("put pointer status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp.Success {
		t.Fatalf("expected success=false, got message=%q", resp.Message)
	}
	if resp.Message == "" {
		t.Fatalf("expected error message")
	}
}

