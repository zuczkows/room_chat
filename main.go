package main

import (
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/zuczkows/room-chat/internal/server"
)

func main() {
	fmt.Println("Starting room-chat app")
	setupApp()

}

func setupApp() {
	logLvl := slog.LevelDebug // note zuczkows - change default to info and move to config
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLvl,
	}))
	manager := server.NewManager(logger)
	go manager.Run()
	http.HandleFunc("/ws", manager.ServeWS)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
