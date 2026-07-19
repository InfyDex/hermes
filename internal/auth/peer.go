package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// PeerAuthMiddleware validates Authorization: Bearer <peer_secret> against the local node secret.
func PeerAuthMiddleware(getSecret func() string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			secret := getSecret()
			if secret == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			token := BearerToken(r.Header.Get("Authorization"))
			if token == "" || !SafeSecretEqual(token, secret) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// BearerToken extracts the token from an Authorization: Bearer header.
func BearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

// SafeSecretEqual compares secrets in constant time without panicking on length mismatch.
func SafeSecretEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// PeerHeartbeatMiddleware validates Bearer token against a registered peer secret.
func PeerHeartbeatMiddleware(lookup func(token string) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := BearerToken(r.Header.Get("Authorization"))
			if token == "" || !lookup(token) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
