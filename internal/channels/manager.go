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

func (c *Channels) Get(channelName string) (*Channel, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	channel, exists := c.channels[channelName]
	if !exists {
		return nil, ErrChannelDoesNotExist
	}
	return channel, nil
}

func (c *Channels) GetOrCreate(channelName string, logger *slog.Logger, sessionManager *user.SessionManager, elastic *elastic.MessageIndexer) *Channel {
	c.mu.Lock()
	defer c.mu.Unlock()
	if channel, exists := c.channels[channelName]; exists {
		return channel
	}
	channel := NewChannel(channelName, logger, sessionManager, elastic)
	c.channels[channelName] = channel
	c.logger.Info("Created new channel", slog.String("channel", channelName))

	return channel
}

func (c *Channels) IsUserAMember(channelName, username string) (bool, error) {
	channel, err := c.Get(channelName)
	if err != nil {
		return false, err
	}

	return channel.HasUser(username), nil
}

func (c *Channels) AddUser(channelName, username string) error {
	channel, err := c.Get(channelName)
	if err != nil {
		return err
	}
	return channel.AddUser(username)
}

func (c *Channels) Delete(channelName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.channels[channelName]; !ok {
		return ErrChannelDoesNotExist
	}
	delete(c.channels, channelName)
	c.logger.Info("Deleted channel", slog.String("channel", channelName))
	return nil
}
