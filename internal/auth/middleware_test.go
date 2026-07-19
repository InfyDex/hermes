package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hermes-scheduler/hermes/internal/auth"
	"github.com/hermes-scheduler/hermes/internal/testutil"
)

func TestBasicAuthMiddleware(t *testing.T) {
	cfg := testutil.TestAuthConfig()
	handler := auth.BasicAuthMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
	req.SetBasicAuth("admin", "secret")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}

	bad := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
	badRR := httptest.NewRecorder()
	handler.ServeHTTP(badRR, bad)
	if badRR.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", badRR.Code)
	}
}

func TestSessionMiddlewareRedirect(t *testing.T) {
	store := testutil.TestSessionStore(t)
	handler := auth.SessionMiddleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc == "" || loc[:7] != "/login?" {
		t.Fatalf("location = %q", loc)
	}
}

func TestSessionMiddlewareWithCookie(t *testing.T) {
	store := testutil.TestSessionStore(t)
	saveRR := httptest.NewRecorder()
	session, _ := store.NewSession("admin", false)
	_ = store.Save(saveRR, session, false)

	handler := auth.SessionMiddleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, ok := auth.SessionFromContext(r.Context())
		if !ok || s.Username != "admin" {
			http.Error(w, "no session", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range saveRR.Result().Cookies() {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSessionMiddlewareAPIUnauthorized(t *testing.T) {
	store := testutil.TestSessionStore(t)
	handler := auth.SessionMiddleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/notifications", nil)
	req.Header.Set("Accept", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
}
