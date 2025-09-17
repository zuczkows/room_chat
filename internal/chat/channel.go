package chat

import (
	"fmt"
	"log/slog"
	"slices"
	"sync"

	"github.com/zuczkows/room-chat/internal/connection"
	"github.com/zuczkows/room-chat/internal/protocol"
)

type Users map[string]*User

type User struct {
	mu      sync.RWMutex
	clients []*connection.Client
}

type Channel struct {
	name   string
	users  Users
	logger *slog.Logger
	mu     sync.RWMutex
}

func NewChannel(name string, logger *slog.Logger) *Channel {
	return &Channel{
		name:   name,
		logger: logger,
		users:  make(Users),
	}
}

func (ch *Channel) Name() string {
	return ch.name
}

func (ch *Channel) AddClient(username string, client *connection.Client) {
	ch.mu.Lock()

	user, exists := ch.users[username]
	if !exists {
		user = &User{
			clients: make([]*connection.Client, 0),
		}
		ch.users[username] = user
	}
	ch.mu.Unlock()

	user.mu.Lock()
	numberOfClients := len(user.clients)
	isFirstClient := numberOfClients == 0
	user.clients = append(user.clients, client)
	user.mu.Unlock()

	if !exists || isFirstClient {
		userJoinedMsg := protocol.Message{
			Type:    protocol.MessageActionSystem,
			Channel: ch.Name(),
			Content: fmt.Sprintf("%s joined the channel", username),
		}
		ch.Broadcast(userJoinedMsg)
	}

}

func (ch *Channel) RemoveClient(username string, client *connection.Client) {
	ch.mu.Lock()

	user, exists := ch.users[username]
	ch.mu.Unlock()
	if !exists {
		return // zuczkows - Maybe it's worth to log some warning??
	}

	user.mu.Lock()
	user.clients = slices.DeleteFunc(user.clients, func(c *connection.Client) bool {
		return c == client
	})
	shouldRemoveUser := len(user.clients) == 0
	user.mu.Unlock()

	if shouldRemoveUser {
		ch.RemoveUser(username)
	}
}

func (ch *Channel) RemoveUser(username string) {
	ch.mu.Lock()
	delete(ch.users, username)
	ch.mu.Unlock()

	leaveMsg := protocol.Message{
		Type:    protocol.MessageActionSystem,
		Channel: ch.Name(),
		Content: fmt.Sprintf("%s left the channel", username),
	}
	ch.Broadcast(leaveMsg)
	ch.logger.Info("User left the channel", slog.String("user", username))
}

func (ch *Channel) Broadcast(message protocol.Message) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	for _, user := range ch.users {
		user.mu.RLock()
		for _, client := range user.clients {
			client.Send() <- message
		}
		user.mu.RUnlock()
	}

}

func (ch *Channel) HasUser(username string) bool {
	ch.mu.RLock()
	user, exists := ch.users[username]
	ch.mu.RUnlock()

	if !exists {
		return false
	}

	user.mu.RLock()
	hasClients := len(user.clients) > 0
	user.mu.RUnlock()

	return hasClients
}

func (ch *Channel) ActiveUsersCount() int {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return len(ch.users)
}
