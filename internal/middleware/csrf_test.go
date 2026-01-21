package middleware

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"realms/internal/auth"
)

func TestCSRF_HeaderTokenOK(t *testing.T) {
	csrf := "csrf_test_token"
	p := auth.Principal{ActorType: auth.ActorTypeSession, UserID: 1, CSRFToken: &csrf}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/x", nil)
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))
	req.Header.Set("X-CSRF-Token", csrf)

	rr := httptest.NewRecorder()
	CSRF()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: got=%d want=%d body=%q", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestCSRF_FormTokenOK(t *testing.T) {
	csrf := "csrf_test_token"
	p := auth.Principal{ActorType: auth.ActorTypeSession, UserID: 1, CSRFToken: &csrf}

	body := strings.NewReader("_csrf=" + csrf + "&a=b")
	req := httptest.NewRequest(http.MethodPost, "http://example.com/x", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	CSRF()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: got=%d want=%d body=%q", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestCSRF_HeaderTokenMismatchFallsBackToForm(t *testing.T) {
	csrf := "csrf_test_token"
	p := auth.Principal{ActorType: auth.ActorTypeSession, UserID: 1, CSRFToken: &csrf}

	body := strings.NewReader("_csrf=" + csrf + "&a=b")
	req := httptest.NewRequest(http.MethodPost, "http://example.com/x", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-CSRF-Token", "wrong")
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	CSRF()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: got=%d want=%d body=%q", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestCSRF_InvalidTokenForbidden(t *testing.T) {
	csrf := "csrf_test_token"
	p := auth.Principal{ActorType: auth.ActorTypeSession, UserID: 1, CSRFToken: &csrf}

	body := strings.NewReader("_csrf=wrong")
	req := httptest.NewRequest(http.MethodPost, "http://example.com/x", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	CSRF()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("unexpected status: got=%d want=%d body=%q", rr.Code, http.StatusForbidden, rr.Body.String())
	}
}

func TestCSRF_MultipartTokenOK(t *testing.T) {
	csrf := "csrf_test_token"
	p := auth.Principal{ActorType: auth.ActorTypeSession, UserID: 1, CSRFToken: &csrf}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("_csrf", csrf); err != nil {
		t.Fatalf("WriteField: %v", err)
	}
	if err := w.WriteField("subject", "hello"); err != nil {
		t.Fatalf("WriteField: %v", err)
	}
	part, err := w.CreateFormFile("attachments", "a.txt")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write([]byte("hello")); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/x", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	CSRF()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: got=%d want=%d body=%q", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestCSRF_FormTooLargeReturns413(t *testing.T) {
	csrf := "csrf_test_token"
	p := auth.Principal{ActorType: auth.ActorTypeSession, UserID: 1, CSRFToken: &csrf}

	// 让 ParseForm 在读取 body 时触发 MaxBytesError。
	payload := strings.Repeat("a", 1024)
	body := strings.NewReader("_csrf=" + csrf + "&pad=" + payload)
	req := httptest.NewRequest(http.MethodPost, "http://example.com/x", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.WithPrincipal(req.Context(), p))

	rr := httptest.NewRecorder()
	req.Body = http.MaxBytesReader(rr, req.Body, 16)
	CSRF()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("unexpected status: got=%d want=%d body=%q", rr.Code, http.StatusRequestEntityTooLarge, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "上传超过大小限制") {
		t.Fatalf("unexpected body: %q", rr.Body.String())
	}
}
