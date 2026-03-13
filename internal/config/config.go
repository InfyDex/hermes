package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Auth     AuthConfig     `yaml:"auth"`
	Database DatabaseConfig `yaml:"database"`
	Logs     LogsConfig     `yaml:"logs"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
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
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
