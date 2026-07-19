package auth

import (
	"crypto/subtle"

	"github.com/hermes-scheduler/hermes/internal/config"
)

// ValidCredentials checks username and password with constant-time comparison.
// Password is always compared even when username mismatches to reduce timing leaks.
func ValidCredentials(cfg *config.AuthConfig, username, password string) bool {
	userMatch := subtle.ConstantTimeCompare([]byte(username), []byte(cfg.Username)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(password), []byte(cfg.Password)) == 1
	return userMatch && passMatch
}
