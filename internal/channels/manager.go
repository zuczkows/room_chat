package channels

import (
	"errors"
	"log/slog"
	"sync"

	"github.com/zuczkows/room-chat/internal/storage"
	"github.com/zuczkows/room-chat/internal/user"
)

var (
	ErrChannelDoesNotExist = errors.New("channel does not exist")
	ErrChannelAlreadyExist = errors.New("channel already exist")
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

func (cm *ChannelManager) Get(channelName string) (*Channel, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	channel, exists := cm.channels[channelName]
	if !exists {
		return nil, ErrChannelDoesNotExist
	}
	return channel, nil
}
func (cm *ChannelManager) GetOrCreate(channelName string, logger *slog.Logger, sessionManager *user.SessionManager, storage *storage.MessageIndexer) *Channel {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if channel, exists := cm.channels[channelName]; exists {
		return channel
	}
	channel := NewChannel(channelName, logger, sessionManager, storage)
	cm.channels[channelName] = channel
	cm.logger.Info("Created new channel", slog.String("channel", channelName))

	return channel
}

func (cm *ChannelManager) IsUserAMember(channelName, username string) bool {
	channel, err := cm.Get(channelName)
	if err != nil {
		cm.logger.Debug("Channel does not exist", slog.String("channel", channelName))
		return false
	}

	return channel.HasUser(username)
}

func (cm *ChannelManager) AddUser(channelName, username string) error {
	channel, err := cm.Get(channelName)
	if err != nil {
		return err
	}
	return channel.AddUser(username)
}

func (cm *ChannelManager) Delete(channelName string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, ok := cm.channels[channelName]; !ok {
		return ErrChannelDoesNotExist
	}
	delete(cm.channels, channelName)
	cm.logger.Info("Deleted channel", slog.String("channel", channelName))
	return nil
}
