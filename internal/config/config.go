package config

import (
	"os"
	"strconv"
)

type Config struct {
	Server   ServerConfig
	Auth     AuthConfig
	Database DatabaseConfig
	Logs     LogsConfig
	Notify   NotifyConfig
}

type ServerConfig struct {
	Port      int
	DomainURL string
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

type DatabaseConfig struct {
	Path string
}

type LogsConfig struct {
	Directory string
}

func DefaultConfig() *Config {
	return &Config{
		Server:   ServerConfig{Port: 4376},
		Auth:     AuthConfig{Username: "admin", Password: "admin"},
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
	if envDomain := os.Getenv("HERMES_DOMAIN_URL"); envDomain != "" {
		cfg.Server.DomainURL = envDomain
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