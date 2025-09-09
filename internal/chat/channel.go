package chat

import (
	"sync"
)

type ClientSet map[Client]bool

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

func (ch *Channel) AddClient(client Client) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.clients[client] = true
}

func (ch *Channel) RemoveClient(client Client) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	delete(ch.clients, client)
}

func (ch *Channel) Broadcast(message Message) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	for client := range ch.clients {
		client.Send() <- message
	}
}

func (ch *Channel) listUsers() ClientSet {
	return ch.clients
}

func (ch *Channel) HasUser(client Client) bool {
	if _, exists := ch.listUsers()[client]; exists {
		return true
	}
	return false
}

func (ch *Channel) ActiveUsers() int {
	return len(ch.listUsers())
}

// Note zuczkows - not sure if it is the right place and proper solution but it avoids import cycle
type Client interface {
	Send() chan<- Message
}
