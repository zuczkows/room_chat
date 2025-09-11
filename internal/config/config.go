package config

import (
	"encoding/json"
	"log/slog"
	"os"
	"slices"
	"strings"
)

type Config struct {
	Server   ServerConfig   `json:"server"`
	Logging  LoggingConfig  `json:"logging"`
	Database DatabaseConfig `json:"database"`
}

type ServerConfig struct {
	Port           int      `json:"port"`
	AllowedOrigins []string `json:"allowed_origins"`
}

type DatabaseConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	DbName   string `json:"dbname"`
	SslMode  string `json:"sslmode"`
}

type LoggingConfig struct {
	Level string `json:"level"`
}

func Load(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *LoggingConfig) GetSlogLevel() slog.Level {
	switch strings.ToLower(c.Level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func (c *ServerConfig) IsOriginAllowed(origin string) bool {
	return slices.Contains(c.AllowedOrigins, origin)
}
