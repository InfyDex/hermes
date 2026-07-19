package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hermes-scheduler/hermes/internal/config"
)

func testSessionStore(t *testing.T) *SessionStore {
	t.Helper()
	store, err := NewSessionStore(&config.SessionConfig{
		Secret:      "test-secret-key-at-least-32-bytes-long",
		TTL:         time.Hour,
		RememberTTL: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewSessionStore: %v", err)
	}
	return store
}

func TestValidCredentials(t *testing.T) {
	cfg := &config.AuthConfig{Username: "admin", Password: "secret"}
	if !ValidCredentials(cfg, "admin", "secret") {
		t.Error("expected valid credentials")
	}
	if ValidCredentials(cfg, "admin", "wrong") {
		t.Error("expected invalid password")
	}
	if ValidCredentials(cfg, "wrong", "secret") {
		t.Error("expected invalid username")
	}
}

func TestSessionSaveAndGet(t *testing.T) {
	store := testSessionStore(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	session, err := store.NewSession("admin", false)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if err := store.Save(rec, session, false); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie")
	}
	req.AddCookie(cookies[0])

	got, ok := store.Get(req)
	if !ok {
		t.Fatal("expected valid session")
	}
	if got.Username != "admin" {
		t.Errorf("username = %q, want admin", got.Username)
	}
}

func TestSessionExpired(t *testing.T) {
	store := testSessionStore(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	session := &SessionData{
		SessionID: "abc",
		Username:  "admin",
		ExpiresAt: time.Now().Add(-time.Hour),
		CSRFToken: "token",
	}
	if err := store.Save(rec, session, false); err != nil {
		t.Fatalf("Save: %v", err)
	}
	req.AddCookie(rec.Result().Cookies()[0])

	if _, ok := store.Get(req); ok {
		t.Error("expected expired session to be rejected")
	}
}

func TestBasicAuthMiddleware(t *testing.T) {
	cfg := &config.AuthConfig{Username: "admin", Password: "secret"}
	handler := BasicAuthMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") != "" {
		t.Error("should not set WWW-Authenticate header")
	}

	req.SetBasicAuth("admin", "secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestSessionMiddlewareRedirect(t *testing.T) {
	store := testSessionStore(t)
	handler := SessionMiddleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", rec.Code)
	}
}

func TestSessionMiddlewareAPIRequest(t *testing.T) {
	store := testSessionStore(t)
	handler := SessionMiddleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/notifications", nil)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestLoginRateLimiter(t *testing.T) {
	limiter := NewLoginRateLimiter()
	ip := "127.0.0.1"
	if !limiter.Allow(ip) {
		t.Fatal("expected first attempt allowed")
	}
	limiter.RecordFailure(ip)
	if !limiter.Allow(ip) {
		t.Error("expected second attempt allowed after single failure")
	}
	for i := 0; i < maxLoginFailures-1; i++ {
		limiter.RecordFailure(ip)
	}
	if limiter.Allow(ip) {
		t.Error("expected IP to be locked out after max failures")
	}
	limiter.Reset(ip)
	if !limiter.Allow(ip) {
		t.Error("expected IP to be allowed after reset")
	}
}

func TestValidateCSRF(t *testing.T) {
	session := &SessionData{CSRFToken: "abc123"}
	if !ValidateCSRF(session, "abc123") {
		t.Error("expected valid CSRF")
	}
	if ValidateCSRF(session, "wrong") {
		t.Error("expected invalid CSRF")
	}
}

func TestRememberMeTTL(t *testing.T) {
	store := testSessionStore(t)
	session, err := store.NewSession("admin", true)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	want := time.Now().Add(24 * time.Hour)
	if session.ExpiresAt.Before(want.Add(-time.Minute)) {
		t.Errorf("remember-me expiry too short: %v", session.ExpiresAt)
	}
}

func TestSessionRotation(t *testing.T) {
	store := testSessionStore(t)
	s1, err := store.NewSession("admin", false)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	s2, err := store.NewSession("admin", false)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if s1.SessionID == s2.SessionID {
		t.Error("expected unique session IDs on each login")
	}
}

func TestSessionClear(t *testing.T) {
	store := testSessionStore(t)
	rec := httptest.NewRecorder()
	store.Clear(rec, false)
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected cleared cookie")
	}
	if cookies[0].MaxAge != -1 {
		t.Errorf("MaxAge = %d, want -1", cookies[0].MaxAge)
	}
}
