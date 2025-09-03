package config

import (
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"time"
)

const (
	WriteWait       = 10 * time.Second
	PongWait        = 60 * time.Second
	PingInterval    = (PongWait * 9) / 10 // Must be less than PongWait
	MaxMessageSize  = 512
	ReadBufferSize  = 1024
	WriteBufferSize = 1024
)

type Config struct {
	Server  ServerConfig  `json:"server"`
	Logging LoggingConfig `json:"logging"`
}

type ServerConfig struct {
	Port           int      `json:"port"`
	AllowedOrigins []string `json:"allowed_origins"`
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
	for _, allowedOrigin := range c.AllowedOrigins {
		if allowedOrigin == origin {
			return true
		}
	}
	return false
}
