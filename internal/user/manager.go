package user

import (
	"errors"
	"log/slog"
	"sync"

	"github.com/zuczkows/room-chat/internal/connection"
)

type SessionManager struct {
	users  map[string]*User
	mu     sync.RWMutex
	logger *slog.Logger
}

func NewSessionManager(logger *slog.Logger) *SessionManager {
	return &SessionManager{
		users:  make(map[string]*User),
		logger: logger,
	}
}

func (um *SessionManager) GetUser(username string) (*User, error) {
	um.mu.RLock()
	defer um.mu.RUnlock()

	user, exists := um.users[username]
	if exists {
		return user, nil
	}
	return nil, errors.New("user not found")
}

func (um *SessionManager) AddClientToUser(username string, client *connection.Client, profile *Profile) *User {
	um.mu.Lock()
	defer um.mu.Unlock()

	user, exists := um.users[username]
	if !exists {
		user = NewUser(profile, um.logger)
		um.users[username] = user
	}

	user.AddClient(client)
	return user
}

func (um *SessionManager) RemoveUser(username string) {
	um.mu.Lock()
	defer um.mu.Unlock()

	if _, exists := um.users[username]; exists {
		delete(um.users, username)
		um.logger.Debug("Removed user", slog.String("user", username))
	}
}

func (um *SessionManager) RemoveClientFromUser(username string, client *connection.Client) {
	um.mu.RLock()
	user, exists := um.users[username]
	um.mu.RUnlock()

	if !exists {
		return
	}

	user.RemoveClient(client)

	if !user.HasConnections() {
		um.RemoveUser(username)
	}
}
