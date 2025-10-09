package user

import (
	"log/slog"
	"sync"

	"github.com/zuczkows/room-chat/internal/connection"
)

type UserManager struct {
	users  map[string]*User
	mu     sync.RWMutex
	logger *slog.Logger
}

func NewUserManager(logger *slog.Logger) *UserManager {
	return &UserManager{
		users:  make(map[string]*User),
		logger: logger,
	}
}

func (um *UserManager) AddUser(user *User) {
	um.mu.Lock()
	um.users[user.Username()] = user
	um.mu.Unlock()
}

func (um *UserManager) GetUser(username string) (*User, bool) {
	um.mu.RLock()
	defer um.mu.RUnlock()

	user, exists := um.users[username]
	return user, exists
}

func (um *UserManager) AddConnectionToUser(username string, client *connection.Client, profile *Profile) *User {
	um.mu.Lock()
	defer um.mu.Unlock()

	user, exists := um.users[username]
	if !exists {
		user = NewUser(profile, um.logger)
		um.users[username] = user
	}

	user.AddConnection(client)
	return user
}

func (um *UserManager) RemoveUser(username string) {
	um.mu.Lock()
	defer um.mu.Unlock()

	if _, exists := um.users[username]; exists {
		delete(um.users, username)
		um.logger.Debug("Removed user", slog.String("user", username))
	}
}

func (um *UserManager) RemoveConnectionFromUser(username string, client *connection.Client) {
	um.mu.RLock()
	user, exists := um.users[username]
	um.mu.RUnlock()

	if !exists {
		return
	}

	user.RemoveConnection(client)

	if !user.HasConnections() {
		um.RemoveUser(username)
	}
}
