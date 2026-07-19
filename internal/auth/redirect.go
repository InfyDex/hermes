package auth

import "strings"

// SafeRedirect returns a same-origin relative path for post-login redirects.
func SafeRedirect(next string) string {
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
	if strings.Contains(strings.ToLower(next), "%2f") {
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
