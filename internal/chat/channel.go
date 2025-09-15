package chat

import (
	"slices"
	"sync"

	"github.com/zuczkows/room-chat/internal/connection"
	"github.com/zuczkows/room-chat/internal/protocol"
)

type Users map[string][]*connection.Client

type Channel struct {
	name  string
	users Users
	mu    sync.RWMutex
}

func NewChannel(name string) *Channel {
	return &Channel{
		name:  name,
		users: make(Users),
	}
}

func (ch *Channel) Name() string {
	return ch.name
}

func (ch *Channel) AddClient(username string, client *connection.Client) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.users[username] = append(ch.users[username], client)
}

func (ch *Channel) RemoveClient(username string, client *connection.Client) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	clients, exists := ch.users[username]
	if !exists {
		return // zuczkows - Maybe it's worth to log some warning??
	}

	ch.users[username] = slices.DeleteFunc(clients, func(c *connection.Client) bool {
		return c == client
	})

	if len(ch.users[username]) == 0 {
		delete(ch.users, username)
	}
}

func (ch *Channel) RemoveUser(username string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	delete(ch.users, username)
}

func (ch *Channel) Broadcast(message protocol.Message) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	for _, clients := range ch.users {
		for _, client := range clients {
			client.Send() <- message
		}
	}
}

func (ch *Channel) HasUser(username string) bool {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	clients, exists := ch.users[username]
	return exists && len(clients) > 0
}

func (ch *Channel) ActiveUsersCount() int {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return len(ch.users)
}
