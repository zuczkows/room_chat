package main

import (
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/zuczkows/room-chat/internal/config"
	"github.com/zuczkows/room-chat/internal/server"
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

	manager := server.NewManager(logger, cfg)
	go manager.Run()

	http.HandleFunc("/ws", manager.ServeWS)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	logger.Info("Starting server", "address", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
