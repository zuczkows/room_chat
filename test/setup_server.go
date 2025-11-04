package test

import (
	"database/sql"
	"log/slog"
	"os"
	"time"

	"github.com/zuczkows/room-chat/internal/config"
	"github.com/zuczkows/room-chat/internal/server"
	"github.com/zuczkows/room-chat/internal/user"
)

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
