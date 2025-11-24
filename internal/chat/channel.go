package chat

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/zuczkows/room-chat/internal/protocol"
	"github.com/zuczkows/room-chat/internal/storage"
	"github.com/zuczkows/room-chat/internal/user"
)

var (
	ErrUserAlreadyExists = errors.New("user already exists")
	ErrNotAMember        = errors.New("user is not a member of a channel")
)

type Channel struct {
	name           string
	users          map[string]struct{}
	logger         *slog.Logger
	mu             sync.RWMutex
	sessionManager *user.SessionManager
	storage        *storage.MessageIndexer
}

func NewChannel(name string, logger *slog.Logger, sessionManager *user.SessionManager, storage *storage.MessageIndexer) *Channel {
	return &Channel{
		name:           name,
		logger:         logger,
		users:          make(map[string]struct{}),
		sessionManager: sessionManager,
		storage:        storage,
	}
}

func (ch *Channel) Name() string {
	return ch.name
}

func (ch *Channel) AddUser(username string) error {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if _, exists := ch.users[username]; exists {
		return ErrUserAlreadyExists
	}
	ch.users[username] = struct{}{}
	ch.logger.Info("User joined channel", slog.String("user", username), slog.String("channel", ch.name))
	return nil
}

func (ch *Channel) RemoveUser(username string) error {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if _, exists := ch.users[username]; !exists {
		return ErrNotAMember
	}

	delete(ch.users, username)
	ch.logger.Info("User left channel", slog.String("user", username), slog.String("channel", ch.name))
	return nil
}

func (ch *Channel) HasUser(username string) bool {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	_, exists := ch.users[username]
	if !exists {
		ch.logger.Debug("User is not a member of a channel", slog.String("username", username))
	}
	return exists
}

func (ch *Channel) GetUsernames() []string {
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

func (ch *Channel) Send(message protocol.Message) {
	users := ch.GetUsernames()
	for _, username := range users {
		ch.sessionManager.SendMessage(username, message)
	}
}

func (ch *Channel) SendWelcomeMessage(username string) {
	joinMsg := protocol.Message{
		Action:  protocol.MessageActionSystem,
		Type:    protocol.MessageTypePush,
		Channel: ch.Name(),
		Push: &protocol.Push{
			Content: fmt.Sprintf("%s joined the channel", username),
		},
	}
	ch.Send(joinMsg)
}

func (ch *Channel) SendLeaveMessage(username string) {
	leaveMsg := protocol.Message{
		Action:  protocol.MessageActionSystem,
		Type:    protocol.MessageTypePush,
		Channel: ch.Name(),
		Push: &protocol.Push{
			Content: fmt.Sprintf("%s left the channel", username),
		},
	}
	ch.Send(leaveMsg)
}

func (ch *Channel) SendMessage(message protocol.Message) {
	messageToSend := protocol.Message{
		Action:  "message",
		Type:    protocol.MessageTypePush,
		Channel: ch.Name(),
		User:    message.User,
		Push: &protocol.Push{
			Content: message.Message,
		},
		CreatedAt: message.CreatedAt,
	}
	ch.Send(messageToSend)
}
