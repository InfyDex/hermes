package auth_test

import (
	"testing"

	"github.com/hermes-scheduler/hermes/internal/auth"
	"github.com/hermes-scheduler/hermes/internal/testutil"
)

func TestValidCredentials(t *testing.T) {
	cfg := testutil.TestAuthConfig()
	if !auth.ValidCredentials(cfg, "admin", "secret") {
		t.Fatal("expected valid credentials")
	}
	if auth.ValidCredentials(cfg, "admin", "wrong") {
		t.Fatal("expected invalid password")
	}
	if auth.ValidCredentials(cfg, "wrong", "secret") {
		t.Fatal("expected invalid username")
	}
}
