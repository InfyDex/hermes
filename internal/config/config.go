package config

import (
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Auth     AuthConfig     `yaml:"auth"`
	Database DatabaseConfig `yaml:"database"`
	Logs     LogsConfig     `yaml:"logs"`
	Notify   NotifyConfig   `yaml:"notify"`
}

type ServerConfig struct {
	Port      int    `yaml:"port"`
	DomainURL string `yaml:"domain_url"` // e.g. https://hermes.edith.in
}

type NotifyConfig struct {
	DiscordWebhookURL string `yaml:"discord_webhook_url"`
	SMTPHost          string `yaml:"smtp_host"`
	SMTPPort          int    `yaml:"smtp_port"`
	SMTPUser          string `yaml:"smtp_user"`
	SMTPPass          string `yaml:"smtp_pass"`
	SMTPFrom          string `yaml:"smtp_from"`
}

type AuthConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type LogsConfig struct {
	Directory string `yaml:"directory"`
}

func DefaultConfig() *Config {
	return &Config{
		Server:   ServerConfig{Port: 8080},
		Auth:     AuthConfig{Username: "admin", Password: "admin"},
		Database: DatabaseConfig{Path: "/data/jobs.db"},
		Logs:     LogsConfig{Directory: "/data/logs"},
	}
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
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