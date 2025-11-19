package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/zuczkows/room-chat/internal/config"
	"github.com/zuczkows/room-chat/internal/database"
	"github.com/zuczkows/room-chat/internal/server"
	"github.com/zuczkows/room-chat/internal/user"
)

func main() {
	fmt.Println("Starting room-chat app")
	setupApp()

}

func setupApp() {
	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.Logging.GetSlogLevel(),
	}))

	dbConfig := database.Config{
		Host:     cfg.Database.Host,
		Port:     cfg.Database.Port,
		User:     cfg.Database.User,
		Password: cfg.Database.Password.String(),
		DBName:   cfg.Database.DbName,
		SSLMode:  cfg.Database.SslMode,
	}
	db, err := database.NewPostgresConnection(dbConfig)
	logger.Info("Starting PostgresConnection", slog.String("host", cfg.Database.Host), slog.Int("port", cfg.Database.Port))
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	userRepo := user.NewPostgresRepository(db)
	userService := user.NewService(userRepo)

	srv := server.NewServer(logger, cfg, userService)
	go srv.Run()
	go srv.Start()

	grpcConfig := server.GrpcConfig{
		Host: cfg.GRPC.Host,
		Port: cfg.GRPC.Port,
	}
	if err := server.StartGrpcServer(userService, logger, grpcConfig); err != nil {
		logger.Error("Failed to serve gRPC", slog.Any("error", err))
		os.Exit(1)
	}
}
