package server

import (
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/zuczkows/room-chat/internal/chat"
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

type ChannelList map[string]*chat.Channel

type Manager struct {
	channels   ChannelList
	clients    ClientList
	register   chan *Client
	unregister chan *Client
	broadcast  chan chat.Message
	mu         sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		channels:   make(map[string]*chat.Channel),
		clients:    make(ClientList),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan chat.Message),
	}
}

func (m *Manager) Run() {
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

func (m *Manager) handleMessage(message chat.Message) {
	switch message.Type {
	case chat.MesageActionJoin:
		m.handleJoinChannel(message)
	case chat.MessageActionLeave:
		m.handleLeaveChannel(message)
	case chat.MessageActionMessage:
		m.handleSendMessage(message)
	}
}

func (m *Manager) handleJoinChannel(message chat.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()

	channelName := message.Channel //NOTE validate Message not empty in struct validate

	if _, exists := m.channels[channelName]; !exists {
		m.channels[channelName] = chat.NewChannel(channelName)
		log.Printf("Created new channel: %s", channelName)
	}

	channel := m.channels[channelName]

	senderClient := m.findClientByUsername(message.User)

	if senderClient != nil {
		channel.AddClient(senderClient)
		senderClient.SetCurrentChannel(channelName)

		userJoinedMsg := chat.Message{
			Type:    "system",
			Channel: channelName,
			User:    message.User,
			Content: fmt.Sprintf("%s joined the channel", message.User),
		}
		channel.Broadcast(userJoinedMsg)
		log.Printf("User %s joined channel %s", message.User, channelName)
	}

}

func (m *Manager) handleSendMessage(message chat.Message) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if channel, exists := m.channels[message.Channel]; exists {
		channel.Broadcast(message)
		log.Printf("Message sent to channel %s by %s", message.Channel, message.User)
	}

	// Notify sender that channel dont exists
}

func (m *Manager) handleLeaveChannel(message chat.Message) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channelName := message.Channel
	if channel, exists := m.channels[channelName]; exists {
		senderClient := m.findClientByUsername(message.User)

		if senderClient != nil {
			channel.RemoveClient(senderClient)
			senderClient.ClearCurrentChannel()

			leaveMsg := chat.Message{
				Type:    "system",
				Channel: channelName,
				Content: fmt.Sprintf("%s left the channel", message.User),
			}
			channel.Broadcast(leaveMsg)

			log.Printf("User %s left channel %s", message.User, channelName)
		}
	}
}

func (m *Manager) ServeWS(w http.ResponseWriter, r *http.Request) {
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
		currentChannel := client.GetCurrentChannel()

		if currentChannel != "" {
			if channel, exists := m.channels[currentChannel]; exists {
				channel.RemoveClient(client)
				userName := client.GetUser()
				leaveMsg := chat.Message{
					Type:    "system",
					Channel: channel.Name(),
					User:    userName,
					Content: fmt.Sprintf("%s left the channel", userName),
				}
				channel.Broadcast(leaveMsg)
			}
		}

		delete(m.clients, client)
		close(client.send)
		log.Println("Client unregistered")
	}
}

func (m *Manager) findClientByUsername(username string) *Client {
	for client := range m.clients {
		if client.GetUser() == username {
			return client
		}
	}
	return nil
}
