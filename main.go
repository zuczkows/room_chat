package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var (
	websocketUpgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow connections from any origin for development
		},
	}
)

type Message struct {
	Type    string `json:"type"`
	Channel string `json:"channel"`
	User    string `json:"user"`
	Content string `json:"content"`
}

type Channel struct {
	name    string
	clients ClientList
	mu      sync.RWMutex
}

func NewChannel(name string) *Channel {
	return &Channel{
		name:    name,
		clients: make(ClientList),
	}
}

func (ch *Channel) addClient(client *Client) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.clients[client] = true
}

func (ch *Channel) removeClient(client *Client) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	delete(ch.clients, client)
}

func (ch *Channel) broadcast(message Message) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	for client := range ch.clients {
		client.send <- message
	}
}

type ChannelList map[string]*Channel

type Manager struct {
	channels   ChannelList
	clients    ClientList
	register   chan *Client
	unregister chan *Client
	broadcast  chan Message
	mu         sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		channels:   make(map[string]*Channel),
		clients:    make(ClientList),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan Message),
	}
}

func (m *Manager) run() {
	for {
		select {
		case client := <-m.register:
			m.addClient(client)
		case client := <-m.unregister:
			m.removeClient(client)
		case message := <-m.broadcast:
			m.handleMessage(message)
		}

	}
}

func (m *Manager) handleMessage(message Message) {
	switch message.Type {
	case "join":
		m.handleJoinChannel(message)
	case "leave":
		m.handleLeaveChannel(message)
	case "message":
		m.handleSendMessage(message)
	}
}

func (m *Manager) handleJoinChannel(message Message) {
	m.mu.Lock()
	defer m.mu.Unlock()

	channelName := message.Channel //NOTE validate Message not empty in struct validate

	if _, exists := m.channels[channelName]; !exists {
		m.channels[channelName] = NewChannel(channelName)
		log.Printf("Created new channel: %s", channelName)
	}

	channel := m.channels[channelName]

	var senderClient *Client
	for client := range m.clients {
		if client.user == message.User {
			senderClient = client
			break
		}
	}

	if senderClient != nil {
		channel.addClient(senderClient)

		userJoinedMsg := Message{
			Type:    "system",
			Channel: channelName,
			Content: fmt.Sprintf("%s joined the channel", message.User),
		}
		channel.broadcast(userJoinedMsg)
		log.Printf("User %s joined channel %s", message.User, channelName)
	}

}

func (m *Manager) handleSendMessage(message Message) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if channel, exists := m.channels[message.Channel]; exists {
		channel.broadcast(message)
		log.Printf("Message sent to channel %s by %s", message.Channel, message.User)
	}

	// Notify sender that channel dont exists
}

func (m *Manager) handleLeaveChannel(message Message) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channelName := message.Channel
	if channel, exists := m.channels[channelName]; exists {
		var senderClient *Client
		for client := range m.clients {
			if client.user == message.User {
				senderClient = client
				break
			}
		}

		if senderClient != nil {
			channel.removeClient(senderClient)

			leaveMsg := Message{
				Type:    "system",
				Channel: channelName,
				Content: fmt.Sprintf("%s left the channel", message.User),
			}
			channel.broadcast(leaveMsg)

			log.Printf("User %s left channel %s", message.User, channelName)
		}
	}
}

func (m *Manager) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	client := NewClient(conn, m)

	m.register <- client
	go client.ReadMessages()
	go client.WriteMessages()

}

func (m *Manager) addClient(client *Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[client] = true
	fmt.Print("Succesfully added a client")
}

func (m *Manager) removeClient(client *Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, exists := m.clients[client]
	if exists {
		for _, channel := range m.channels {
			channel.removeClient(client)
		}
		delete(m.clients, client)
		close(client.send)
		log.Println("Client unregistered")
	}
}

func main() {
	fmt.Println("Starting room-chat app")
	setupApp()

}

func setupApp() {
	manager := NewManager()
	go manager.run()
	http.HandleFunc("/ws", manager.serveWS)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
