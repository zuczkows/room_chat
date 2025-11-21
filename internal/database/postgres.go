package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func NewPostgresConnection(cfg Config) (*sql.DB, error) {
	// NOTE(zuczkows): debug logs for Dockerfile
	fmt.Printf("DB_HOST: %s\n", cfg.Host)
	fmt.Printf("DB_PORT: %d\n", cfg.Port)
	fmt.Printf("DB_USER: %s\n", cfg.User)

	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("cannot open db: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(1 * time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("cannot ping db: %w", err)
	}

	return db, nil
}
