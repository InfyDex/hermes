package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/hermes-scheduler/hermes/internal/config"
	"github.com/hermes-scheduler/hermes/internal/testutil"
)

func TestDefaultConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	if cfg.Server.Port != 4376 {
		t.Fatalf("port = %d", cfg.Server.Port)
	}
	if cfg.Auth.Username != "admin" {
		t.Fatalf("username = %q", cfg.Auth.Username)
	}
	if cfg.Session.TTL != 24*time.Hour {
		t.Fatalf("session TTL = %v", cfg.Session.TTL)
	}
}

func TestLoadRequiresSessionSecret(t *testing.T) {
	cleanup := testutil.SetTestEnv(t)
	defer cleanup()
	os.Unsetenv("HERMES_SESSION_SECRET")

	if _, err := config.Load(); err == nil {
		t.Fatal("expected error without session secret")
	}
}

func TestLoadWithEnvOverrides(t *testing.T) {
	cleanup := testutil.SetTestEnv(t)
	defer cleanup()

	os.Setenv("HERMES_PORT", "8080")
	os.Setenv("HERMES_USERNAME", "user1")
	os.Setenv("HERMES_PASSWORD", "pass1")
	os.Setenv("HERMES_DOMAIN_URL", "https://hermes.example")
	os.Setenv("HERMES_SERVER_NAME", "prod")
	os.Setenv("HERMES_NODE_ID", "node-1")
	os.Setenv("HERMES_SESSION_TTL", "2h")
	os.Setenv("HERMES_SESSION_REMEMBER_TTL", "48h")
	os.Setenv("HERMES_SECURE_COOKIES", "true")
	os.Setenv("HERMES_TRUST_PROXY", "true")
	os.Setenv("HERMES_DISCORD_WEBHOOK_URL", "https://discord.example/hook")
	os.Setenv("HERMES_SMTP_HOST", "smtp.example")
	os.Setenv("HERMES_SMTP_PORT", "465")
	os.Setenv("HERMES_SMTP_USER", "mail@example")
	os.Setenv("HERMES_SMTP_PASS", "smtp-pass")
	os.Setenv("HERMES_SMTP_FROM", "from@example")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Fatalf("port = %d", cfg.Server.Port)
	}
	if cfg.Auth.Username != "user1" || cfg.Auth.Password != "pass1" {
		t.Fatalf("auth = %+v", cfg.Auth)
	}
	if cfg.Session.TTL != 2*time.Hour {
		t.Fatalf("TTL = %v", cfg.Session.TTL)
	}
	if !cfg.Session.SecureCookies || !cfg.Server.TrustProxy {
		t.Fatal("expected secure cookies and trust proxy")
	}
	if cfg.Notify.DiscordWebhookURL == "" || cfg.Notify.SMTPHost == "" {
		t.Fatalf("notify = %+v", cfg.Notify)
	}
	if cfg.Fleet.NodeID != "node-1" {
		t.Fatalf("fleet node id = %q", cfg.Fleet.NodeID)
	}
}

func TestLoadIgnoresInvalidPort(t *testing.T) {
	cleanup := testutil.SetTestEnv(t)
	defer cleanup()
	os.Setenv("HERMES_PORT", "not-a-number")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 4376 {
		t.Fatalf("port = %d", cfg.Server.Port)
	}
}
