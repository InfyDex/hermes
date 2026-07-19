package auth

import "testing"

func TestSafeRedirect(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "/"},
		{"/dashboard", "/dashboard"},
		{"/jobs/1", "/jobs/1"},
		{"//evil.com", "/"},
		{"/\\@evil.com", "/"},
		{"/%2f%2fevil.com", "/"},
		{"/login", "/"},
		{"/login?next=/", "/"},
		{"/%252f%252fevil.com", "/"},
		{"/jobs/" + string(make([]byte, 513)), "/"},
	}

	for _, tt := range tests {
		got := SafeRedirect(tt.in)
		if got != tt.want {
			t.Errorf("SafeRedirect(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
