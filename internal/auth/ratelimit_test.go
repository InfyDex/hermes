package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hermes-scheduler/hermes/internal/auth"
	"github.com/hermes-scheduler/hermes/internal/testutil"
)

func TestClientIPIPv6(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "[2001:db8::1]:12345"
	if got := auth.ClientIP(req, false); got != "2001:db8::1" {
		t.Fatalf("ip = %q", got)
	}
}

func TestSessionSecureCookiesConfig(t *testing.T) {
	cfg := testutil.TestSessionConfig()
	cfg.SecureCookies = true
	store, err := auth.NewSessionStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if !store.IsSecureRequest(req) {
		t.Fatal("expected secure cookies config to force secure")
	}
}

func TestLoginRateLimiter(t *testing.T) {
	limiter := auth.NewLoginRateLimiter()
	ip := "10.0.0.1"
	for i := 0; i < 4; i++ {
		if !limiter.Allow(ip) {
			t.Fatalf("expected allow on attempt %d", i+1)
		}
		limiter.RecordFailure(ip)
	}
	if !limiter.Allow(ip) {
		t.Fatal("expected allow before lockout threshold")
	}
	limiter.RecordFailure(ip)
	if limiter.Allow(ip) {
		t.Fatal("expected lockout after 5 failures")
	}
	limiter.Reset(ip)
	if !limiter.Allow(ip) {
		t.Fatal("expected allow after reset")
	}
}

func TestClientIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.5:12345"
	if got := auth.ClientIP(req, false); got != "192.168.1.5" {
		t.Fatalf("ip = %q", got)
	}

	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	if got := auth.ClientIP(req, true); got != "1.2.3.4" {
		t.Fatalf("ip with proxy = %q", got)
	}
	if got := auth.ClientIP(req, false); got != "192.168.1.5" {
		t.Fatalf("ip without proxy = %q", got)
	}

	req.Header.Del("X-Forwarded-For")
	req.Header.Set("X-Real-IP", "9.9.9.9")
	if got := auth.ClientIP(req, true); got != "9.9.9.9" {
		t.Fatalf("real ip = %q", got)
	}
}

func TestLoginCSRF(t *testing.T) {
	rr := httptest.NewRecorder()
	token, err := auth.SetLoginCSRF(rr, false)
	if err != nil || token == "" {
		t.Fatalf("SetLoginCSRF: token=%q err=%v", token, err)
	}

	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	for _, c := range rr.Result().Cookies() {
		req.AddCookie(c)
	}
	_ = req.ParseForm()
	req.Form.Set("csrf_token", token)
	if !auth.ValidateLoginCSRF(req) {
		t.Fatal("expected valid login CSRF")
	}

	req.Form.Set("csrf_token", "wrong")
	if auth.ValidateLoginCSRF(req) {
		t.Fatal("expected invalid login CSRF")
	}

	clearRR := httptest.NewRecorder()
	auth.ClearLoginCSRF(clearRR, false)
}

func TestSafeRedirect(t *testing.T) {
	tests := map[string]string{
		"":                "/",
		"/jobs":           "/jobs",
		"//evil.com":      "/",
		"http://evil.com": "/",
		"/login":          "/",
		"/login?x=1":      "/",
		`/\evil`:          "/",
		"/jobs%2f../":     "/jobs/../",
	}
	for input, want := range tests {
		if got := auth.SafeRedirect(input); got != want {
			t.Fatalf("SafeRedirect(%q) = %q, want %q", input, got, want)
		}
	}
}
