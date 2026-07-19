package auth

import (
	"net/url"
	"strings"
)

// SafeRedirect returns a same-origin relative path for post-login redirects.
func SafeRedirect(next string) string {
	next = decodeRepeatedly(next)
	if next == "" {
		return "/"
	}
	if !strings.HasPrefix(next, "/") {
		return "/"
	}
	if strings.HasPrefix(next, "//") {
		return "/"
	}
	if strings.Contains(next, `\`) || strings.Contains(next, "@") {
		return "/"
	}
	if strings.Contains(strings.ToLower(next), "%") {
		return "/"
	}
	if strings.HasPrefix(next, "/login") {
		return "/"
	}
	if len(next) > 512 {
		return "/"
	}
	return next
}

func decodeRepeatedly(value string) string {
	current := value
	for i := 0; i < 5; i++ {
		decoded, err := url.PathUnescape(current)
		if err != nil {
			return current
		}
		if decoded == current {
			break
		}
		current = decoded
	}
	return current
}
