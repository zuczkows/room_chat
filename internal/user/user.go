package user

import (
	"log/slog"
	"sync"

	"github.com/zuczkows/room-chat/internal/connection"
	"github.com/zuczkows/room-chat/internal/protocol"
)

type User struct {
	profile     *Profile
	connections map[*connection.Client]struct{}
	mu          sync.RWMutex
	logger      *slog.Logger
}

func NewUser(profile *Profile, logger *slog.Logger) *User {
	return &User{
		profile:     profile,
		connections: make(map[*connection.Client]struct{}),
		logger:      logger,
	}
}

func (u *User) Username() string {
	return u.profile.Username
}

func (u *User) AddConnection(client *connection.Client) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.connections[client] = struct{}{}

	u.logger.Debug("Connection added to user",
		slog.String("user", u.Username()),
		slog.Int("total_connections", len(u.connections)))
}

func (u *User) RemoveConnection(client *connection.Client) {
	u.mu.Lock()
	defer u.mu.Unlock()

	if _, exists := u.connections[client]; exists {
		delete(u.connections, client)
		u.logger.Debug("Connection removed from user",
			slog.String("User", u.Username()),
			slog.Int("total_connections", len(u.connections)))
	}
}

func (u *User) SendEvent(message protocol.Message) {
	u.mu.RLock()
	connectionsCopy := make([]*connection.Client, 0, len(u.connections))
	for conn := range u.connections {
		connectionsCopy = append(connectionsCopy, conn)
	}
	u.mu.RUnlock()

	for _, conn := range connectionsCopy {
		select {
		case conn.Send() <- message:
		default:
			u.logger.Warn("Failed to send event to user connection",
				slog.String("user", u.Username()))
		}
	}
}

func (u *User) HasConnections() bool {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return len(u.connections) > 0
}
