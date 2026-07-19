package auth

import "errors"

var ErrInvalidSessionSecret = errors.New("HERMES_SESSION_SECRET must be at least 32 bytes")
