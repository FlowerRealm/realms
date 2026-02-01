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

func TestUserTokensCRUD_SessionCookie(t *testing.T) {
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
	cookies := rr.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("expected set-cookie")
	}

	sessionCookie := ""
	for _, c := range cookies {
		if c.Name == cookieName {
			sessionCookie = c.String()
			break
		}
	}
	if sessionCookie == "" {
		t.Fatalf("expected session cookie %q", cookieName)
	}

	// create token
	createBody, _ := json.Marshal(map[string]any{
		"name": "t1",
	})
	req = httptest.NewRequest(http.MethodPost, "http://example.com/api/token", bytes.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			TokenID   int64   `json:"token_id"`
			Token     string  `json:"token"`
			TokenHint *string `json:"token_hint"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("json.Unmarshal create: %v", err)
	}
	if !created.Success {
		t.Fatalf("create expected success, got message=%q", created.Message)
	}
	if created.Data.TokenID <= 0 {
		t.Fatalf("expected token_id > 0, got %d", created.Data.TokenID)
	}
	if created.Data.Token == "" {
		t.Fatalf("expected token")
	}

	// list tokens
	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/token", nil)
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
		Data    []any  `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("json.Unmarshal list: %v", err)
	}
	if !listResp.Success || len(listResp.Data) != 1 {
		t.Fatalf("expected 1 token, got success=%v len=%d msg=%q", listResp.Success, len(listResp.Data), listResp.Message)
	}

	// reveal token (active)
	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/token/"+strconv.FormatInt(created.Data.TokenID, 10)+"/reveal", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("reveal status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("reveal expected Cache-Control no-store, got %q", got)
	}
	var revealed struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			TokenID int64  `json:"token_id"`
			Token   string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &revealed); err != nil {
		t.Fatalf("json.Unmarshal reveal: %v", err)
	}
	if !revealed.Success || revealed.Data.TokenID != created.Data.TokenID || revealed.Data.Token != created.Data.Token {
		t.Fatalf("reveal unexpected resp: %#v", revealed)
	}

	// simulate old token (missing token_plain)
	if _, err := db.Exec(`UPDATE user_tokens SET token_plain=NULL WHERE id=?`, created.Data.TokenID); err != nil {
		t.Fatalf("clear token_plain: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/token/"+strconv.FormatInt(created.Data.TokenID, 10)+"/reveal", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("reveal(old) status=%d body=%s", rr.Code, rr.Body.String())
	}
	var revealOld struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &revealOld); err != nil {
		t.Fatalf("json.Unmarshal reveal(old): %v", err)
	}
	if revealOld.Success || revealOld.Message == "" {
		t.Fatalf("reveal(old) expected error, got %#v", revealOld)
	}

	// revoke token
	req = httptest.NewRequest(http.MethodPost, "http://example.com/api/token/"+strconv.FormatInt(created.Data.TokenID, 10)+"/revoke", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke status=%d body=%s", rr.Code, rr.Body.String())
	}
	var okResp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &okResp); err != nil {
		t.Fatalf("json.Unmarshal revoke: %v", err)
	}
	if !okResp.Success {
		t.Fatalf("revoke expected success, got message=%q", okResp.Message)
	}

	// reveal token (revoked: should fail)
	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/token/"+strconv.FormatInt(created.Data.TokenID, 10)+"/reveal", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("reveal(revoked) status=%d body=%s", rr.Code, rr.Body.String())
	}
	var revealRevoked struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &revealRevoked); err != nil {
		t.Fatalf("json.Unmarshal reveal(revoked): %v", err)
	}
	if revealRevoked.Success || revealRevoked.Message == "" {
		t.Fatalf("reveal(revoked) expected error, got %#v", revealRevoked)
	}

	// rotate token
	req = httptest.NewRequest(http.MethodPost, "http://example.com/api/token/"+strconv.FormatInt(created.Data.TokenID, 10)+"/rotate", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("rotate status=%d body=%s", rr.Code, rr.Body.String())
	}
	var rotated struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			TokenID int64  `json:"token_id"`
			Token   string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &rotated); err != nil {
		t.Fatalf("json.Unmarshal rotate: %v", err)
	}
	if !rotated.Success || rotated.Data.Token == "" || rotated.Data.TokenID != created.Data.TokenID {
		t.Fatalf("rotate unexpected resp: %#v", rotated)
	}

	// reveal token (after rotate)
	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/token/"+strconv.FormatInt(created.Data.TokenID, 10)+"/reveal", nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("reveal(after rotate) status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("reveal(after rotate) expected Cache-Control no-store, got %q", got)
	}
	var revealAfterRotate struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			TokenID int64  `json:"token_id"`
			Token   string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &revealAfterRotate); err != nil {
		t.Fatalf("json.Unmarshal reveal(after rotate): %v", err)
	}
	if !revealAfterRotate.Success || revealAfterRotate.Data.TokenID != created.Data.TokenID || revealAfterRotate.Data.Token != rotated.Data.Token {
		t.Fatalf("reveal(after rotate) unexpected resp: %#v", revealAfterRotate)
	}

	// delete token
	req = httptest.NewRequest(http.MethodDelete, "http://example.com/api/token/"+strconv.FormatInt(created.Data.TokenID, 10), nil)
	req.Header.Set("Realms-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Cookie", sessionCookie)
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &okResp); err != nil {
		t.Fatalf("json.Unmarshal delete: %v", err)
	}
	if !okResp.Success {
		t.Fatalf("delete expected success, got message=%q", okResp.Message)
	}
}
