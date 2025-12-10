package channels

import (
	"errors"
	"log/slog"
	"sync"

	"github.com/zuczkows/room-chat/internal/elastic"
	"github.com/zuczkows/room-chat/internal/user"
)

var (
	ErrChannelDoesNotExist = errors.New("channel does not exist")
	ErrChannelAlreadyExist = errors.New("channel already exist")
)

type Channels struct {
	channels map[string]*Channel
	mu       sync.RWMutex
	logger   *slog.Logger
}

func NewChannels(logger *slog.Logger) *Channels {
	return &Channels{
		channels: make(map[string]*Channel),
		logger:   logger,
	}
}

func (cm *Channels) Get(channelName string) (*Channel, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	channel, exists := cm.channels[channelName]
	if !exists {
		return nil, ErrChannelDoesNotExist
	}
	return channel, nil
}

func (cm *Channels) GetOrCreate(channelName string, logger *slog.Logger, sessionManager *user.SessionManager, elastic *elastic.MessageIndexer) *Channel {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if channel, exists := cm.channels[channelName]; exists {
		return channel
	}
	channel := NewChannel(channelName, logger, sessionManager, elastic)
	cm.channels[channelName] = channel
	cm.logger.Info("Created new channel", slog.String("channel", channelName))

	return channel
}

func (cm *Channels) IsUserAMember(channelName, username string) (bool, error) {
	channel, err := cm.Get(channelName)
	if err != nil {
		return false, err
	}

	return channel.HasUser(username), nil
}

func (cm *Channels) AddUser(channelName, username string) error {
	channel, err := cm.Get(channelName)
	if err != nil {
		return err
	}
	return channel.AddUser(username)
}

func (cm *Channels) Delete(channelName string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, ok := cm.channels[channelName]; !ok {
		return ErrChannelDoesNotExist
	}
	delete(cm.channels, channelName)
	cm.logger.Info("Deleted channel", slog.String("channel", channelName))
	return nil
}
