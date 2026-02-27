package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	"realms/internal/store"
)

func newTestSQLiteStore(t *testing.T) (*store.Store, func()) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "realms.db") + "?_busy_timeout=1000"
	db, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	if err := store.EnsureSQLiteSchema(db); err != nil {
		_ = db.Close()
		t.Fatalf("EnsureSQLiteSchema: %v", err)
	}

	st := store.New(db)
	st.SetDialect(store.DialectSQLite)

	return st, func() { _ = db.Close() }
}

func newTestEngine(t *testing.T, st *store.Store) (*gin.Engine, string) {
	t.Helper()

	gin.SetMode(gin.TestMode)

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
		PersonalMode:      false,
		FrontendIndexPage: []byte("<!doctype html><html><body>INDEX</body></html>"),
	})

	return engine, cookieName
}

func loginCookie(t *testing.T, engine *gin.Engine, cookieName, login, password string) string {
	t.Helper()

	loginBody, _ := json.Marshal(map[string]any{
		"login":    login,
		"password": password,
	})
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/user/login", bytes.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", rr.Code, rr.Body.String())
	}

	for _, c := range rr.Result().Cookies() {
		if c.Name == cookieName {
			return c.String()
		}
	}
	t.Fatalf("expected session cookie %q", cookieName)
	return ""
}
