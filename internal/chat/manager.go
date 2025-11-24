package chat

import (
	"errors"
	"log/slog"
	"sync"

	"github.com/zuczkows/room-chat/internal/storage"
	"github.com/zuczkows/room-chat/internal/user"
)

var (
	ErrChannelDoesNotExists = errors.New("channel does not exist")
	ErrChannelAlreadyExists = errors.New("channel already exist")
)

type ChannelManager struct {
	channels map[string]*Channel
	mu       sync.RWMutex
	logger   *slog.Logger
}

func NewChannelManager(logger *slog.Logger) *ChannelManager {
	return &ChannelManager{
		channels: make(map[string]*Channel),
		logger:   logger,
	}
}

func (cm *ChannelManager) GetChannel(channelName string) (*Channel, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	channel, exists := cm.channels[channelName]
	if !exists {
		return nil, ErrChannelDoesNotExists
	}
	return channel, nil
}
func (cm *ChannelManager) AddChannel(channelName string, logger *slog.Logger, sessionManager *user.SessionManager, storage *storage.MessageIndexer) (*Channel, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if _, exists := cm.channels[channelName]; exists {
		return nil, ErrChannelAlreadyExists
	}
	channel := NewChannel(channelName, logger, sessionManager, storage)
	cm.channels[channelName] = channel
	cm.logger.Info("Created new channel", slog.String("channel", channelName))

	return channel, nil
}

func (cm *ChannelManager) IsUserAMember(channelName, username string) bool {
	cm.mu.RLock()
	channel, exists := cm.channels[channelName]
	cm.mu.RUnlock()
	if !exists {
		cm.logger.Debug("Channel does not exists in ChannelManager", slog.String("channel", channelName))
		return false
	}

	return channel.HasUser(username)
}

func (cm *ChannelManager) AddUserToChannel(channelName, username string) error {
	cm.mu.RLock()
	channel, exists := cm.channels[channelName]
	cm.mu.RUnlock()

	if !exists {
		return ErrChannelDoesNotExists
	}
	return channel.AddUser(username)
}

func (cm *ChannelManager) DeleteChannel(channelName string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, ok := cm.channels[channelName]; !ok {
		return ErrChannelDoesNotExists
	}
	delete(cm.channels, channelName)
	cm.logger.Info("Deleted channel", slog.String("channel", channelName))
	return nil
}
