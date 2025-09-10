package chat

import (
	"sync"

	"github.com/zuczkows/room-chat/internal/connection"
	"github.com/zuczkows/room-chat/internal/protocol"
)

type ClientSet map[*connection.Client]bool

type Channel struct {
	name    string
	clients ClientSet
	mu      sync.RWMutex
}

func NewChannel(name string) *Channel {
	return &Channel{
		name:    name,
		clients: make(ClientSet),
	}
}

func (ch *Channel) Name() string {
	return ch.name
}

func (ch *Channel) AddClient(client *connection.Client) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.clients[client] = true
}

func (ch *Channel) RemoveClient(client *connection.Client) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	delete(ch.clients, client)
}

func (ch *Channel) Broadcast(message protocol.Message) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	for client := range ch.clients {
		client.Send() <- message
	}
}

func (ch *Channel) HasUser(client *connection.Client) bool {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	if _, exists := ch.clients[client]; exists {
		return true
	}
	return false
}

func (ch *Channel) ActiveUsers() int {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return len(ch.clients)
}
