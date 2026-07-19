package auth

import (
	"crypto/sha256"
	"net/http"
	"time"

	"github.com/gorilla/securecookie"

	"github.com/hermes-scheduler/hermes/internal/config"
)

const sessionCookieName = "hermes_session"

// SessionData holds signed session cookie values.
type SessionData struct {
	SessionID string
	Username  string
	ExpiresAt time.Time
	Remember  bool
	CSRFToken string
}

// SessionStore manages signed session cookies.
type SessionStore struct {
	cfg    *config.SessionConfig
	cookie *securecookie.SecureCookie
}

func NewSessionStore(cfg *config.SessionConfig) (*SessionStore, error) {
	if len(cfg.Secret) < 32 {
		return nil, ErrInvalidSessionSecret
	}
	hashKey := []byte(cfg.Secret)
	sum := sha256.Sum256(hashKey)
	blockKey := sum[:]
	return &SessionStore{
		cfg:    cfg,
		cookie: securecookie.New(hashKey, blockKey),
	}, nil
}

func (s *SessionStore) NewSession(username string, remember bool) (*SessionData, error) {
	sessionID, err := GenerateSessionID()
	if err != nil {
		return nil, err
	}
	csrf, err := GenerateCSRFToken()
	if err != nil {
		return nil, err
	}

	ttl := s.cfg.TTL
	if remember {
		ttl = s.cfg.RememberTTL
	}

	data := &SessionData{
		SessionID: sessionID,
		Username:  username,
		ExpiresAt: time.Now().Add(ttl),
		Remember:  remember,
		CSRFToken: csrf,
	}
	return data, nil
}

func (s *SessionStore) Save(w http.ResponseWriter, data *SessionData, secure bool) error {
	encoded, err := s.cookie.Encode(sessionCookieName, data)
	if err != nil {
		return err
	}

	maxAge := int(time.Until(data.ExpiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    encoded,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure || s.cfg.SecureCookies,
	})
	return nil
}

func (s *SessionStore) Get(r *http.Request) (*SessionData, bool) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return nil, false
	}

	var data SessionData
	if err := s.cookie.Decode(sessionCookieName, c.Value, &data); err != nil {
		return nil, false
	}
	if time.Now().After(data.ExpiresAt) {
		return nil, false
	}
	return &data, true
}

func (s *SessionStore) Clear(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure || s.cfg.SecureCookies,
	})
}

func (s *SessionStore) IsSecureRequest(r *http.Request) bool {
	if s.cfg.SecureCookies {
		return true
	}
	if r.TLS != nil {
		return true
	}
	return r.Header.Get("X-Forwarded-Proto") == "https"
}

func ValidateCSRF(session *SessionData, token string) bool {
	if session == nil || token == "" {
		return false
	}
	return session.CSRFToken == token
}
