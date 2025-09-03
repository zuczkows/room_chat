package chat

import (
	"fmt"
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
	fmt.Println("Removing user")
	delete(ch.clients, client)
}

func (ch *Channel) Broadcast(message Message) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	for client := range ch.clients {
		client.Send() <- message
	}
}

func (ch *Channel) ListUsers() ClientSet {
	return ch.clients
}

// Note zuczkows - not sure if it is the right place and proper solution but it avoids import cycle
type Client interface {
	Send() chan<- Message
}
