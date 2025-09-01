package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/zuczkows/room-chat/internal/server"
)

func main() {
	fmt.Println("Starting room-chat app")
	setupApp()

}

func setupApp() {
	manager := server.NewManager()
	go manager.Run()
	http.HandleFunc("/ws", manager.ServeWS)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
