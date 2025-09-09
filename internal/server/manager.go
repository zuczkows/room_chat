package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/zuczkows/room-chat/internal/chat"
	"github.com/zuczkows/room-chat/internal/config"
)

func getUpgraderWithConfig(cfg *config.Config) websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:  config.ReadBufferSize,
		WriteBufferSize: config.WriteBufferSize,
		CheckOrigin:     createOriginChecker(cfg),
	}
}

func createOriginChecker(cfg *config.Config) func(*http.Request) bool {
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		return cfg.Server.IsOriginAllowed(origin)
	}
}

type ChannelList map[string]*chat.Channel

type Manager struct {
	channels        ChannelList
	clients         ClientList
	register        chan *Client
	unregister      chan *Client
	dispatchMessage chan chat.Message
	mu              sync.RWMutex
	logger          *slog.Logger
	config          *config.Config
}

func NewManager(logger *slog.Logger, cfg *config.Config) *Manager {
	return &Manager{
		channels:        make(map[string]*chat.Channel),
		clients:         make(ClientList),
		register:        make(chan *Client),
		unregister:      make(chan *Client),
		dispatchMessage: make(chan chat.Message),
		logger:          logger,
		config:          cfg,
	}
}

func (m *Manager) Run() {
	for {
		select {
		case client := <-m.register:
			m.addClient(client)
		case client := <-m.unregister:
			m.removeClient(client)
		case message := <-m.dispatchMessage:
			m.handleMessage(message)
		}

	}
}

func (m *Manager) handleMessage(message chat.Message) {
	switch message.Type {
	case chat.MesageActionJoin:
		m.handleJoinChannel(message)
	case chat.MessageActionLeave:
		m.handleLeaveChannel(message)
	case chat.MessageActionText:
		m.handleSendMessage(message)
	}
}

func (m *Manager) handleJoinChannel(message chat.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()

	channelName := message.Channel
	senderClient := m.findClientByUsername(message.User)

	channel, exists := m.channels[channelName]
	if exists {
		if channel.HasUser(senderClient) {
			userAlreadyInChannelMsg := chat.Message{
				Type:    chat.MessageActionSystem,
				Content: fmt.Sprintf("You are already in a channel: %s", channelName),
			}
			senderClient.send <- userAlreadyInChannelMsg
			return
		}
	} else {
		m.channels[channelName] = chat.NewChannel(channelName)
		channel = m.channels[channelName]
		m.logger.Info("Created new channel", slog.String("channel", channelName))
	}

	if senderClient != nil {
		channel.AddClient(senderClient)
		senderClient.SetCurrentChannel(channelName)

		userJoinedMsg := chat.Message{
			Type:    chat.MessageActionSystem,
			Channel: channelName,
			User:    message.User,
			Content: fmt.Sprintf("%s joined the channel", message.User),
		}
		channel.Broadcast(userJoinedMsg)
		m.logger.Info("User joined channel",
			slog.String("user", message.User),
			slog.String("channel", channelName))
	}

}

func (m *Manager) handleSendMessage(message chat.Message) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	senderClient := m.findClientByUsername(message.User)

	channel, exists := m.channels[message.Channel]
	if exists {
		if channel.HasUser(senderClient) {
			channel.Broadcast(message)
			m.logger.Info("Message sent to channel",
				slog.String("channel", message.Channel),
				slog.String("user", message.User))
			return
		} else {
			userNotInChannel := chat.Message{
				Type:    chat.MessageActionSystem,
				Content: fmt.Sprintf("You are not a member of this channel: %s", message.Channel),
			}
			senderClient.send <- userNotInChannel
		}
	} else {
		channelNotExists := chat.Message{
			Type:    chat.MessageActionSystem,
			Content: fmt.Sprintf("Channel does not exists: %s", message.Channel),
		}
		senderClient.send <- channelNotExists
	}
}

func (m *Manager) handleLeaveChannel(message chat.Message) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channelName := message.Channel
	if channel, exists := m.channels[channelName]; exists {
		senderClient := m.findClientByUsername(message.User)

		if senderClient != nil && channel.HasUser(senderClient) {
			channel.RemoveClient(senderClient)
			senderClient.ClearCurrentChannel()

			leaveMsg := chat.Message{
				Type:    chat.MessageActionSystem,
				Channel: channelName,
				Content: fmt.Sprintf("%s left the channel", message.User),
			}
			channel.Broadcast(leaveMsg)

			m.logger.Info("User left channel",
				slog.String("user", message.User),
				slog.String("channel", channelName))
		}
		if activeUsers := channel.ActiveUsers(); activeUsers == 0 {
			delete(m.channels, channelName)
			m.logger.Info("Channel deleted", slog.String("channel", channelName))
		}

	}
}

func (m *Manager) ServeWS(w http.ResponseWriter, r *http.Request) {
	upgrader := getUpgraderWithConfig(m.config)
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		m.logger.Error("WebSocket upgrade failed", slog.Any("error", err))
		return
	}

	client := NewClient(conn, m.unregister, m.dispatchMessage, m.logger)

	m.register <- client
	go client.ReadMessages()
	go client.WriteMessages()

}

func (m *Manager) addClient(client *Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[client] = true
	m.logger.Info("Client added", slog.Int("total_clients", len(m.clients)))
}

func (m *Manager) removeClient(client *Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, exists := m.clients[client]
	if exists {
		currentChannel := client.GetCurrentChannel()

		if currentChannel != "" {
			if channel, exists := m.channels[currentChannel]; exists {
				channel.RemoveClient(client)
				userName := client.GetUser()
				leaveMsg := chat.Message{
					Type:    chat.MessageActionSystem,
					Channel: channel.Name(),
					User:    userName,
					Content: fmt.Sprintf("%s left the channel", userName),
				}
				channel.Broadcast(leaveMsg)
			}
		}

		delete(m.clients, client)
		close(client.send)
		m.logger.Info("Client unregistered")
	}
}

func (m *Manager) findClientByUsername(username string) *Client {
	for client := range m.clients {
		if client.GetUser() == username {
			return client
		}
	}
	return nil
}
