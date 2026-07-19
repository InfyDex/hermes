package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server   ServerConfig
	Auth     AuthConfig
	Session  SessionConfig
	Database DatabaseConfig
	Logs     LogsConfig
	Notify   NotifyConfig
}

type ServerConfig struct {
	Port       int
	DomainURL  string
	ServerName string
}

type NotifyConfig struct {
	DiscordWebhookURL string
	SMTPHost          string
	SMTPPort          int
	SMTPUser          string
	SMTPPass          string
	SMTPFrom          string
}

type AuthConfig struct {
	Username string
	Password string
}

type SessionConfig struct {
	Secret        string
	TTL           time.Duration
	RememberTTL   time.Duration
	SecureCookies bool
}

type DatabaseConfig struct {
	Path string
}

type LogsConfig struct {
	Directory string
}

func DefaultConfig() *Config {
	return &Config{
		Server:  ServerConfig{Port: 4376},
		Auth:    AuthConfig{Username: "admin", Password: "admin"},
		Session: SessionConfig{TTL: 24 * time.Hour, RememberTTL: 720 * time.Hour},
		Database: DatabaseConfig{Path: "/data/jobs.db"},
		Logs:     LogsConfig{Directory: "/data/logs"},
	}
}

func Load() (*Config, error) {
	cfg := DefaultConfig()

	if envPort := os.Getenv("HERMES_PORT"); envPort != "" {
		if port, err := strconv.Atoi(envPort); err == nil {
			cfg.Server.Port = port
		}
	}

	if envUser := os.Getenv("HERMES_USERNAME"); envUser != "" {
		cfg.Auth.Username = envUser
	}
	if envPass := os.Getenv("HERMES_PASSWORD"); envPass != "" {
		cfg.Auth.Password = envPass
	}
	if envSecret := os.Getenv("HERMES_SESSION_SECRET"); envSecret != "" {
		cfg.Session.Secret = envSecret
	}
	if envTTL := os.Getenv("HERMES_SESSION_TTL"); envTTL != "" {
		if d, err := time.ParseDuration(envTTL); err == nil {
			cfg.Session.TTL = d
		}
	}
	if envRememberTTL := os.Getenv("HERMES_SESSION_REMEMBER_TTL"); envRememberTTL != "" {
		if d, err := time.ParseDuration(envRememberTTL); err == nil {
			cfg.Session.RememberTTL = d
		}
	}
	if os.Getenv("HERMES_SECURE_COOKIES") == "true" {
		cfg.Session.SecureCookies = true
	}
	if len(cfg.Session.Secret) < 32 {
		return nil, fmt.Errorf("HERMES_SESSION_SECRET is required and must be at least 32 bytes")
	}
	if envDomain := os.Getenv("HERMES_DOMAIN_URL"); envDomain != "" {
		cfg.Server.DomainURL = envDomain
	}
	if envServerName := os.Getenv("HERMES_SERVER_NAME"); envServerName != "" {
		cfg.Server.ServerName = envServerName
	}
	if envDiscord := os.Getenv("HERMES_DISCORD_WEBHOOK_URL"); envDiscord != "" {
		cfg.Notify.DiscordWebhookURL = envDiscord
	}
	if envSMTPHost := os.Getenv("HERMES_SMTP_HOST"); envSMTPHost != "" {
		cfg.Notify.SMTPHost = envSMTPHost
	}
	if envSMTPPort := os.Getenv("HERMES_SMTP_PORT"); envSMTPPort != "" {
		if port, err := strconv.Atoi(envSMTPPort); err == nil {
			cfg.Notify.SMTPPort = port
		}
	}
	if envSMTPUser := os.Getenv("HERMES_SMTP_USER"); envSMTPUser != "" {
		cfg.Notify.SMTPUser = envSMTPUser
	}
	if envSMTPPass := os.Getenv("HERMES_SMTP_PASS"); envSMTPPass != "" {
		cfg.Notify.SMTPPass = envSMTPPass
	}
	if envSMTPFrom := os.Getenv("HERMES_SMTP_FROM"); envSMTPFrom != "" {
		cfg.Notify.SMTPFrom = envSMTPFrom
	}
	return cfg, nil
}
