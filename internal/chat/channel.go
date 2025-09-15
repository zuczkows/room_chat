package chat

import (
	"slices"
	"sync"

	"github.com/zuczkows/room-chat/internal/connection"
	"github.com/zuczkows/room-chat/internal/protocol"
)

type ClientSet map[*connection.Client]bool
type Users []string

type Channel struct {
	name        string
	userClients map[string][]*connection.Client
	mu          sync.RWMutex
}

func NewChannel(name string) *Channel {
	return &Channel{
		name:        name,
		userClients: make(map[string][]*connection.Client),
	}
}

func (ch *Channel) Name() string {
	return ch.name
}

func (ch *Channel) AddClient(username string, client *connection.Client) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.userClients[username] = append(ch.userClients[username], client)
}

func (ch *Channel) RemoveClient(username string, client *connection.Client) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	clients, exists := ch.userClients[username]
	if !exists {
		return
	}

	ch.userClients[username] = slices.DeleteFunc(clients, func(c *connection.Client) bool {
		return c == client
	})

	if len(ch.userClients[username]) == 0 {
		delete(ch.userClients, username)
	}
}

func (ch *Channel) RemoveUser(username string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	delete(ch.userClients, username)
}

func (ch *Channel) Broadcast(message protocol.Message) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	for _, clients := range ch.userClients {
		for _, client := range clients {
			client.Send() <- message
		}
	}
}

func (ch *Channel) HasUser(username string) bool {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	clients, exists := ch.userClients[username]
	return exists && len(clients) > 0
}

func (ch *Channel) ActiveUsersCount() int {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return len(ch.userClients)
}
