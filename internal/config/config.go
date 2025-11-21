package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
)

const envPrefix = "#env:"

type Secret string

func (s *Secret) UnmarshalJSON(data []byte) error {
	var key string
	if err := json.Unmarshal(data, &key); err != nil {
		return err
	}

	if !strings.HasPrefix(key, envPrefix) {
		*s = Secret(key)
		return nil
	}

	env := strings.TrimPrefix(key, envPrefix)
	value, exists := os.LookupEnv(env)
	if !exists {
		return fmt.Errorf("env var %s doesn't exist", env)
	}
	*s = Secret(value)

	return nil
}

func (s *Secret) String() string {
	return string(*s)
}

type Config struct {
	Server   ServerConfig   `json:"server"`
	Logging  LoggingConfig  `json:"logging"`
	Database DatabaseConfig `json:"database"`
	GRPC     GrpcConfig     `json:"grpc"`
}

type ServerConfig struct {
	Port           int      `json:"port"`
	AllowedOrigins []string `json:"allowed_origins"`
}

type DatabaseConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password Secret `json:"password"`
	DbName   string `json:"dbname"`
	SslMode  string `json:"sslmode"`
}

type GrpcConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
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
