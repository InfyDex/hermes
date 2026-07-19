package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hermes-scheduler/hermes/internal/auth"
	"github.com/hermes-scheduler/hermes/internal/testutil"
)

func TestNewSessionStoreInvalidSecret(t *testing.T) {
	_, err := auth.NewSessionStore(testutil.TestSessionConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	short := testutil.TestSessionConfig()
	short.Secret = "short"
	if _, err := auth.NewSessionStore(short); err != auth.ErrInvalidSessionSecret {
		t.Fatalf("err = %v", err)
	}
}

func TestSessionSaveGetClear(t *testing.T) {
	store := testutil.TestSessionStore(t)
	rr := httptest.NewRecorder()
	session, err := store.NewSession("admin", false)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if err := store.Save(rr, session, false); err != nil {
		t.Fatalf("Save: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rr.Result().Cookies() {
		req.AddCookie(c)
	}
	got, ok := store.Get(req)
	if !ok || got.Username != "admin" {
		t.Fatalf("Get = %+v, ok=%v", got, ok)
	}

	clearRR := httptest.NewRecorder()
	store.Clear(clearRR, false)
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range clearRR.Result().Cookies() {
		req2.AddCookie(c)
	}
	if _, ok := store.Get(req2); ok {
		t.Fatal("expected session cleared")
	}
}

func TestSessionExpired(t *testing.T) {
	store := testutil.TestSessionStore(t)
	rr := httptest.NewRecorder()
	session := &auth.SessionData{
		SessionID: "id",
		Username:  "admin",
		ExpiresAt: time.Now().Add(-time.Hour),
		CSRFToken: "token",
	}
	if err := store.Save(rr, session, false); err != nil {
		t.Fatalf("Save: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rr.Result().Cookies() {
		req.AddCookie(c)
	}
	if _, ok := store.Get(req); ok {
		t.Fatal("expected expired session rejected")
	}
}

func TestValidateCSRF(t *testing.T) {
	session := &auth.SessionData{CSRFToken: "abc"}
	if !auth.ValidateCSRF(session, "abc") {
		t.Fatal("expected valid CSRF")
	}
	if auth.ValidateCSRF(session, "wrong") {
		t.Fatal("expected invalid CSRF")
	}
	if auth.ValidateCSRF(nil, "abc") {
		t.Fatal("expected nil session invalid")
	}
}

func TestIsSecureRequest(t *testing.T) {
	store := testutil.TestSessionStore(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	if !store.IsSecureRequest(req) {
		t.Fatal("expected secure via forwarded proto")
	}
}

func TestNewSessionRememberTTL(t *testing.T) {
	store := testutil.TestSessionStore(t)
	session, err := store.NewSession("admin", true)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if !session.Remember {
		t.Fatal("expected remember flag")
	}
}
