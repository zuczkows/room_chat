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
	c "github.com/zuczkows/room-chat/internal/channels"
	"github.com/zuczkows/room-chat/internal/config"
	"github.com/zuczkows/room-chat/internal/elastic"
	"github.com/zuczkows/room-chat/internal/server"
	"github.com/zuczkows/room-chat/internal/user"
)

const (
	grpcAddr = "0.0.0.0"
	grpcPort = 50051
)

var (
	users     *user.Users
	esStorage *elastic.MessageIndexer
	channels  *c.Channels
	db        *sql.DB
	esClient  *es7.Client
	grpcErrCh <-chan error
)

func TestMain(m *testing.M) {
	var cleanupPG func()
	var cleanupES func()
	var err error

	db, cleanupPG, err = SetupPG()
	if err != nil {
		log.Fatalf("Error setting up PG: %v", err)
		os.Exit(1)
	}
	defer cleanupPG()
	esClient, cleanupES, err = SetupES()
	if err != nil {
		log.Fatalf("Error setting up ES: %v", err)
		os.Exit(1)
	}
	defer cleanupES()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	users, esStorage, channels = SetupServer(db, logger)
	_, grpcErrCh = SetupGrpc(logger, esStorage, channels)

	exitCode := m.Run()
	os.Exit(exitCode)
}

func SetupPG() (*sql.DB, func(), error) {
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

	return db, func() {
		err := testcontainers.TerminateContainer(postgresContainer)
		if err != nil {
			log.Printf("failed to terminate postgres container: %s", err)
		}
		log.Printf("Postgres test container terminated")
	}, nil
}

func SetupES() (*es7.Client, func(), error) {
	esCtx, esCancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer esCancel()
	elasticsearchContainer, err := esc.Run(esCtx, "docker.elastic.co/elasticsearch/elasticsearch:7.9.2",
		elasticsearch.WithPassword("foo"))
	if err != nil {
		log.Printf("failed to start es container: %s", err)
		return nil, nil, nil
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
		return nil, nil, nil
	}

	return esClient, func() {
		err = testcontainers.TerminateContainer(elasticsearchContainer)
		if err != nil {
			log.Printf("failed to terminate ES container: %s", err)
		}
		log.Printf("ES test container terminated")
	}, nil
}

func SetupServer(db *sql.DB, logger *slog.Logger) (*user.Users, *elastic.MessageIndexer, *c.Channels) {
	userRepo := user.NewPostgresRepository(db)
	users := user.NewUsers(userRepo)
	logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	channels := c.NewChannels(logger)
	elastic := elastic.NewMessageIndexer(esClient, logger, "messages")
	elastic.CreateIndex()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
			AllowedOrigins: []string{
				"http://localhost:8080",
				"",
			},
		},
	}

	server := server.NewServer(logger, cfg, users, elastic, channels)
	go server.Run()
	go server.Start()

	time.Sleep(time.Millisecond * 20)
	return users, elastic, channels
}

func SetupGrpc(logger *slog.Logger, elastic *elastic.MessageIndexer, channels *c.Channels) (*server.GrpcServer, <-chan error) {
	grpcServer := server.NewGrpcServer(users, logger, elastic, channels)

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
