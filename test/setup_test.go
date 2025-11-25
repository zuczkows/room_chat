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

	es7 "github.com/elastic/go-elasticsearch/v7"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/elasticsearch"
	esc "github.com/testcontainers/testcontainers-go/modules/elasticsearch"
	pgc "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/zuczkows/room-chat/internal/chat"
	"github.com/zuczkows/room-chat/internal/config"
	"github.com/zuczkows/room-chat/internal/server"
	"github.com/zuczkows/room-chat/internal/storage"
	"github.com/zuczkows/room-chat/internal/user"
)

const (
	grpcAddr = "0.0.0.0"
	grpcPort = 50051
)

var (
	userService    *user.Service
	esStorage      *storage.MessageIndexer
	channelManager *chat.ChannelManager
	db             *sql.DB
	esClient       *es7.Client
	grpcErrCh      <-chan error
)

func TestMain(m *testing.M) {
	var cleanup func()
	var err error

	db, esClient, cleanup, err = SetupDB()
	if err != nil {
		log.Fatalf("Error setting up database: %v", err)
		os.Exit(1)
	}
	defer cleanup()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	userService, esStorage, channelManager = SetupServer(db, logger)
	_, grpcErrCh = SetupGrpc(logger, esStorage, channelManager)

	exitCode := m.Run()
	os.Exit(exitCode)
}

func SetupDB() (*sql.DB, *es7.Client, func(), error) {
	ctx := context.Background()

	migrationPath := filepath.Join("..", "migrations", "001_create_users_table.up.sql")
	postgresContainer, err := pgc.Run(ctx,
		"postgres:16-alpine",
		pgc.WithInitScripts(migrationPath),
		pgc.BasicWaitStrategies(),
	)
	log.Printf("Postgres container is running")
	if err != nil {
		log.Printf("failed to start postgres container: %s", err)
		return nil, nil, nil, nil
	}

	connStr, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Printf("failed to return connection string for postgres database: %s", err)
		return nil, nil, nil, err
	}
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		log.Printf("failed to connect to database: %s", err)
		return nil, nil, nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, nil, nil, fmt.Errorf("cannot ping db: %w", err)
	}

	esCtx, esCancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer esCancel()
	elasticsearchContainer, err := esc.Run(esCtx, "docker.elastic.co/elasticsearch/elasticsearch:7.9.2",
		elasticsearch.WithPassword("foo"))
	if err != nil {
		log.Printf("failed to start es container: %s", err)
		return nil, nil, nil, nil
	}
	log.Printf("ES container is running")
	esConfig := es7.Config{
		Addresses: []string{
			elasticsearchContainer.Settings.Address,
		},
		Username: "elastic",
		Password: elasticsearchContainer.Settings.Password,
		CACert:   elasticsearchContainer.Settings.CACert,
	}
	esClient, err := es7.NewClient(esConfig)
	if err != nil {
		log.Printf("error creating the client: %s", err)
		return nil, nil, nil, nil
	}

	return db, esClient, func() {
		err := testcontainers.TerminateContainer(postgresContainer)
		if err != nil {
			log.Printf("failed to terminate postgres container: %s", err)
		}
		err = testcontainers.TerminateContainer(elasticsearchContainer)
		if err != nil {
			log.Printf("failed to terminate ES container: %s", err)
		}
		log.Printf("Test container terminated")
	}, nil
}

func SetupServer(db *sql.DB, logger *slog.Logger) (*user.Service, *storage.MessageIndexer, *chat.ChannelManager) {
	userRepo := user.NewPostgresRepository(db)
	userService := user.NewService(userRepo)
	logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	channelManager := chat.NewChannelManager(logger)
	storage := storage.NewMessageIndexer(esClient, logger)
	storage.CreateIndex()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
			AllowedOrigins: []string{
				"http://localhost:8080",
				"",
			},
		},
	}

	server := server.NewServer(logger, cfg, userService, storage, channelManager)
	go server.Run()
	go server.Start()

	time.Sleep(time.Millisecond * 20)
	return userService, storage, channelManager
}

func SetupGrpc(logger *slog.Logger, storage *storage.MessageIndexer, channelManager *chat.ChannelManager) (*server.GrpcServer, <-chan error) {
	grpcServer := server.NewGrpcServer(userService, logger, storage, channelManager)

	errCh := make(chan error, 1)

	go func() {
		fmt.Printf("Starting gRPC server address: %s, port: %d", grpcAddr, grpcPort)
		grpcConfig := server.GrpcConfig{
			Host: grpcAddr,
			Port: grpcPort,
		}
		errCh <- grpcServer.Start(grpcConfig)
	}()

	return grpcServer, errCh
}
