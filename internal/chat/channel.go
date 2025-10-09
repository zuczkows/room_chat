package chat

import (
	"log/slog"
	"sync"
)

type Channel struct {
	name   string
	users  map[string]struct{}
	logger *slog.Logger
	mu     sync.RWMutex
}

func NewChannel(name string, logger *slog.Logger) *Channel {
	return &Channel{
		name:   name,
		logger: logger,
		users:  make(map[string]struct{}),
	}
}

func (ch *Channel) Name() string {
	return ch.name
}

func (ch *Channel) AddUser(username string) bool {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if _, exists := ch.users[username]; exists {
		return false
	}
	ch.users[username] = struct{}{}
	ch.logger.Info("User joined channel",
		slog.String("user", username),
		slog.String("channel", ch.name))
	return true
}

func (ch *Channel) RemoveUser(username string) bool {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if _, exists := ch.users[username]; !exists {
		return false
	}

	delete(ch.users, username)
	ch.logger.Info("User left channel",
		slog.String("user", username),
		slog.String("channel", ch.name))
	return true
}

func (ch *Channel) HasUser(username string) bool {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	_, exists := ch.users[username]
	return exists
}

func (ch *Channel) GetUsers() []string {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	users := make([]string, 0, len(ch.users))
	for username := range ch.users {
		users = append(users, username)
	}
	return users
}

func (ch *Channel) ActiveUsersCount() int {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return len(ch.users)
}
