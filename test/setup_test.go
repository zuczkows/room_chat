//go:build integration

package test

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	pgc "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/zuczkows/room-chat/internal/config"
	"github.com/zuczkows/room-chat/internal/server"
	"github.com/zuczkows/room-chat/internal/user"
)

var (
	userService *user.Service
	db          *sql.DB
)

func TestMain(m *testing.M) {
	var cleanup func()
	var err error

	db, cleanup, err = SetupDB()
	if err != nil {
		log.Fatalf("Error setting up database: %v", err)
		os.Exit(1)
	}
	defer cleanup()

	userService = SetupServer(db)
	exitCode := m.Run()
	os.Exit(exitCode)
}

func SetupDB() (*sql.DB, func(), error) {
	ctx := context.Background()

	migrationPath := filepath.Join("..", "migrations", "001_create_users_table.up.sql")
	postgresContainer, err := pgc.Run(ctx,
		"postgres:16-alpine",
		pgc.WithInitScripts(migrationPath),
		pgc.BasicWaitStrategies(),
	)
	if err != nil {
		log.Printf("failed to start container: %s", err)
		return nil, nil, nil
	}

	connStr, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Printf("failed to return connection string for postgres database: %s", err)
		return nil, nil, err
	}
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		log.Printf("failed to connect to database: %s", err)
		return nil, nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, nil, fmt.Errorf("cannot ping db: %w", err)
	}
	log.Printf("Db is running")
	return db, func() {
		err := testcontainers.TerminateContainer(postgresContainer)
		if err != nil {
			log.Printf("failed to terminate container: %s", err)
		}
		log.Printf("Test container terminated")
	}, nil
}

func SetupServer(db *sql.DB) *user.Service {
	userRepo := user.NewPostgresRepository(db)
	userService := user.NewService(userRepo)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
			AllowedOrigins: []string{
				"http://localhost:8080",
				"",
			},
		},
	}

	server := server.NewServer(logger, cfg, userService)
	go server.Run()
	go server.Start()

	time.Sleep(time.Millisecond * 20)
	return userService
}
