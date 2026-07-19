package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	maxLoginFailures = 5
	loginLockout     = 15 * time.Minute
)

type loginAttempt struct {
	failures    int
	lockedUntil time.Time
}

// LoginRateLimiter limits failed login attempts per client IP.
type LoginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string]*loginAttempt
}

func NewLoginRateLimiter() *LoginRateLimiter {
	return &LoginRateLimiter{
		attempts: make(map[string]*loginAttempt),
	}
}

func (l *LoginRateLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	att, ok := l.attempts[ip]
	if !ok {
		return true
	}
	if now.Before(att.lockedUntil) {
		return false
	}
	if att.failures >= maxLoginFailures {
		delete(l.attempts, ip)
	}
	return true
}

func (l *LoginRateLimiter) RecordFailure(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	att, ok := l.attempts[ip]
	if !ok {
		l.attempts[ip] = &loginAttempt{failures: 1, lockedUntil: time.Now().Add(loginLockout)}
		return
	}
	att.failures++
	if att.failures >= maxLoginFailures {
		att.lockedUntil = time.Now().Add(loginLockout)
	}
}

func (l *LoginRateLimiter) Reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, ip)
}

func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := netSplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func netSplitHostPort(addr string) (string, string, error) {
	if strings.HasPrefix(addr, "[") {
		if i := strings.LastIndex(addr, "]:"); i >= 0 {
			return addr[1:i], addr[i+2:], nil
		}
		return strings.Trim(addr, "[]"), "", nil
	}
	host, port, found := strings.Cut(addr, ":")
	if found {
		return host, port, nil
	}
	return addr, "", nil
}

// CSRF token helpers stored in session cookie payload.
func GenerateCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func GenerateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
