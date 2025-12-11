package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/elastic/go-elasticsearch/v7"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/zuczkows/room-chat/internal/channels"
	"github.com/zuczkows/room-chat/internal/config"
	"github.com/zuczkows/room-chat/internal/elastic"
	"github.com/zuczkows/room-chat/internal/postgres"
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

	pgConfig := postgres.Config{
		Host:     cfg.Database.Host,
		Port:     cfg.Database.Port,
		User:     cfg.Database.User,
		Password: cfg.Database.Password.String(),
		DBName:   cfg.Database.DbName,
		SSLMode:  cfg.Database.SslMode,
	}
	db, err := postgres.NewPostgresConnection(pgConfig)
	logger.Info("Starting PostgresConnection", slog.String("host", cfg.Database.Host), slog.Int("port", cfg.Database.Port))
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	userRepo := user.NewPostgresRepository(db)
	users := user.NewUsers(userRepo)

	es, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{
			fmt.Sprintf("http://%s:%d", cfg.Elasticsearch.Host, cfg.Elasticsearch.Port),
		},
	})
	if err != nil {
		log.Fatalf("Failed to create Elasticsearch client: %v", err)
	}
	elastic := elastic.NewMessageIndexer(es, logger, cfg.Elasticsearch.Index)
	channels := channels.NewChannels(logger)
	srv := server.NewServer(logger, cfg, users, elastic, channels)
	go srv.Run()
	go srv.Start()
	grpcConfig := server.GrpcConfig{
		Host: cfg.GRPC.Host,
		Port: cfg.GRPC.Port,
	}
	grpcServer := server.NewGrpcServer(users, logger, elastic, channels)
	if err := grpcServer.Start(grpcConfig); err != nil {
		logger.Error("Failed to serve gRPC", slog.Any("error", err))
		os.Exit(1)
	}
}
