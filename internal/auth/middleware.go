package auth

import (
	"context"
	"net/http"
	"net/url"

	"github.com/hermes-scheduler/hermes/internal/config"
)

type contextKey string

const sessionContextKey contextKey = "hermes_session"

// SessionFromContext returns the authenticated session from the request context.
func SessionFromContext(ctx context.Context) (*SessionData, bool) {
	s, ok := ctx.Value(sessionContextKey).(*SessionData)
	return s, ok
}

// BasicAuthMiddleware protects API routes with HTTP Basic Auth.
// Does not set WWW-Authenticate to avoid triggering the browser login dialog.
func BasicAuthMiddleware(cfg *config.AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, pass, ok := r.BasicAuth()
			if !ok || !ValidCredentials(cfg, user, pass) {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SessionMiddleware protects web routes with a signed session cookie.
func SessionMiddleware(store *SessionStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, ok := store.Get(r)
			if !ok {
				if isAPIRequest(r) {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
				nextURL := url.QueryEscape(r.URL.RequestURI())
				http.Redirect(w, r, "/login?next="+nextURL, http.StatusSeeOther)
				return
			}

			ctx := context.WithValue(r.Context(), sessionContextKey, session)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func isAPIRequest(r *http.Request) bool {
	if r.Header.Get("Accept") == "application/json" {
		return true
	}
	if r.Header.Get("Sec-Fetch-Dest") == "empty" {
		return true
	}
	return false
}
