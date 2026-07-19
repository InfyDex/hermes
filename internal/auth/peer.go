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
			token := bearerToken(r.Header.Get("Authorization"))
			if token == "" || !safeSecretEqual(token, secret) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

// safeSecretEqual compares secrets in constant time without panicking on length mismatch.
func safeSecretEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
