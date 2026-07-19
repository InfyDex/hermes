package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPeerAuthMiddlewareRejectsWrongLengthToken(t *testing.T) {
	handler := PeerAuthMiddleware(func() string { return "secret-token" })(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/fleet/handshake", nil)
	req.Header.Set("Authorization", "Bearer short")
	rr := httptest.NewRecorder()

	defer func() {
		if recover() != nil {
			t.Fatal("peer auth must not panic on token length mismatch")
		}
	}()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}
