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

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	"realms/internal/auth"
	"realms/internal/store"
)

func TestAnnouncements_UserFlow(t *testing.T) {
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
	userID, err := st.CreateUser(ctx, "u@example.com", "u", pwHash, store.UserRoleUser)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if userID <= 0 {
		t.Fatalf("expected userID > 0")
	}

	announcementID, err := st.CreateAnnouncement(ctx, "t1", "body1", store.AnnouncementStatusPublished)
	if err != nil {
		t.Fatalf("CreateAnnouncement: %v", err)
	}
	if announcementID <= 0 {
		t.Fatalf("expected announcementID > 0")
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
		"login":    "u@example.com",
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

	// list announcements (unread)
	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/announcements?limit=10", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
	}
	var listResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			UnreadCount int64 `json:"unread_count"`
			Items       []struct {
				ID    int64  `json:"id"`
				Read  bool   `json:"read"`
				Title string `json:"title"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("json.Unmarshal list: %v", err)
	}
	if !listResp.Success {
		t.Fatalf("expected success, got message=%q", listResp.Message)
	}
	if listResp.Data.UnreadCount != 1 {
		t.Fatalf("expected unread_count=1, got %d", listResp.Data.UnreadCount)
	}
	if len(listResp.Data.Items) != 1 || listResp.Data.Items[0].ID != announcementID || listResp.Data.Items[0].Read {
		t.Fatalf("unexpected list items: %#v", listResp.Data.Items)
	}

	// view detail (should mark read best-effort)
	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/announcements/"+strconv.FormatInt(announcementID, 10), nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", rr.Code, rr.Body.String())
	}

	// list again (read)
	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/announcements", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list2 status=%d body=%s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("json.Unmarshal list2: %v", err)
	}
	if !listResp.Success || listResp.Data.UnreadCount != 0 || len(listResp.Data.Items) != 1 || !listResp.Data.Items[0].Read {
		t.Fatalf("unexpected list2 resp: %#v", listResp)
	}

	// mark read explicitly
	req = httptest.NewRequest(http.MethodPost, "http://example.com/api/announcements/"+strconv.FormatInt(announcementID, 10)+"/read", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("read status=%d body=%s", rr.Code, rr.Body.String())
	}
	var okResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &okResp); err != nil {
		t.Fatalf("json.Unmarshal read: %v", err)
	}
	if !okResp.Success {
		t.Fatalf("expected success, got message=%q", okResp.Message)
	}
}
