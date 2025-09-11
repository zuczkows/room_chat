package server

import (
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zuczkows/room-chat/internal/chat"
	"github.com/zuczkows/room-chat/internal/config"
	"github.com/zuczkows/room-chat/internal/connection"
	"github.com/zuczkows/room-chat/internal/handlers"
	"github.com/zuczkows/room-chat/internal/middleware"
	"github.com/zuczkows/room-chat/internal/protocol"
	"github.com/zuczkows/room-chat/internal/user"
)

const (
	ReadBufferSize  = 1024
	WriteBufferSize = 1024
)

func getUpgraderWithConfig(cfg *config.Config) websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:  ReadBufferSize,
		WriteBufferSize: WriteBufferSize,
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
type ClientList map[*connection.Client]bool

type Manager struct {
	channels        ChannelList
	clients         ClientList
	register        chan *connection.Client
	unregister      chan *connection.Client
	dispatchMessage chan protocol.Message
	mu              sync.RWMutex
	logger          *slog.Logger
	config          *config.Config
	server          *http.Server
	userService     *user.Service
}

func NewManager(logger *slog.Logger, cfg *config.Config, userService *user.Service) *Manager {
	return &Manager{
		channels:        make(map[string]*chat.Channel),
		clients:         make(ClientList),
		register:        make(chan *connection.Client),
		unregister:      make(chan *connection.Client),
		dispatchMessage: make(chan protocol.Message),
		logger:          logger,
		config:          cfg,
		userService:     userService,
	}
}

func (m *Manager) Mount() http.Handler {
	r := http.NewServeMux()
	userHandler := handlers.NewUserHandler(m.userService, m.logger)
	authMiddleware := middleware.NewAuthMiddleware(m.userService, m.logger)

	r.HandleFunc("GET /ws", m.ServeWS)
	r.HandleFunc("POST /api/register", userHandler.HandleRegister)
	r.Handle("PUT /api/profile", authMiddleware.BasicAuth(http.HandlerFunc(userHandler.HandleUpdate)))
	return r
}

func (m *Manager) StartServ(mux http.Handler) {
	m.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", m.config.Server.Port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	m.logger.Info("Starting server", "address", m.config.Server.Port)
	log.Fatal(m.server.ListenAndServe())
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

func (m *Manager) handleMessage(message protocol.Message) {
	switch message.Type {
	case protocol.MesageActionJoin:
		m.handleJoinChannel(message)
	case protocol.MessageActionLeave:
		m.handleLeaveChannel(message)
	case protocol.MessageActionText:
		m.handleSendMessage(message)
	}
}

func (m *Manager) handleJoinChannel(message protocol.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()

	channelName := message.Channel
	senderClient := m.findClientByUsername(message.User)

	channel, exists := m.channels[channelName]
	if exists {
		if channel.HasUser(senderClient) {
			userAlreadyInChannelMsg := protocol.Message{
				Type:    protocol.MessageActionSystem,
				Content: fmt.Sprintf("You are already in a channel: %s", channelName),
			}
			senderClient.Send() <- userAlreadyInChannelMsg
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

		userJoinedMsg := protocol.Message{
			Type:    protocol.MessageActionSystem,
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

func (m *Manager) handleSendMessage(message protocol.Message) {
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
			userNotInChannel := protocol.Message{
				Type:    protocol.MessageActionSystem,
				Content: fmt.Sprintf("You are not a member of this channel: %s", message.Channel),
			}
			senderClient.Send() <- userNotInChannel
		}
	} else {
		channelNotExists := protocol.Message{
			Type:    protocol.MessageActionSystem,
			Content: fmt.Sprintf("Channel does not exists: %s", message.Channel),
		}
		senderClient.Send() <- channelNotExists
	}
}

func (m *Manager) handleLeaveChannel(message protocol.Message) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channelName := message.Channel
	senderClient := m.findClientByUsername(message.User)

	channel, exists := m.channels[channelName]
	if !exists {
		if senderClient != nil {
			errorMsg := protocol.Message{
				Type:    protocol.MessageActionSystem,
				Content: fmt.Sprintf("channel %s does not exist", channelName),
			}
			senderClient.Send() <- errorMsg
		}
		return
	}

	if senderClient != nil && channel.HasUser(senderClient) {
		channel.RemoveClient(senderClient)
		senderClient.ClearCurrentChannel()

		leaveMsg := protocol.Message{
			Type:    protocol.MessageActionSystem,
			Channel: channelName,
			Content: fmt.Sprintf("%s left the channel", message.User),
		}
		channel.Broadcast(leaveMsg)

		m.logger.Info("User left channel",
			slog.String("user", message.User),
			slog.String("channel", channelName))

		if activeUsersCount := channel.ActiveUsersCount(); activeUsersCount == 0 {
			delete(m.channels, channelName)
			m.logger.Info("Channel deleted", slog.String("channel", channelName))
		}
	} else if senderClient != nil {
		errorMsg := protocol.Message{
			Type:    protocol.MessageActionSystem,
			Content: fmt.Sprintf("You are not a member of channel '%s'", channelName),
		}
		senderClient.Send() <- errorMsg
	}
}

func (m *Manager) ServeWS(w http.ResponseWriter, r *http.Request) {
	upgrader := getUpgraderWithConfig(m.config)
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		m.logger.Error("WebSocket upgrade failed", slog.Any("error", err))
		return
	}

	client := connection.NewClient(conn, m.unregister, m.dispatchMessage, m.logger)

	m.register <- client
	go client.ReadMessages()
	go client.WriteMessages()

}

func (m *Manager) addClient(client *connection.Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[client] = true
	m.logger.Info("Client added", slog.Int("total_clients", len(m.clients)))
}

func (m *Manager) removeClient(client *connection.Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, exists := m.clients[client]
	if exists {
		currentChannel := client.GetCurrentChannel()

		if currentChannel != "" {
			if channel, exists := m.channels[currentChannel]; exists {
				channel.RemoveClient(client)
				userName := client.GetUser()
				leaveMsg := protocol.Message{
					Type:    protocol.MessageActionSystem,
					Channel: channel.Name(),
					User:    userName,
					Content: fmt.Sprintf("%s left the channel", userName),
				}
				channel.Broadcast(leaveMsg)
			}
		}

		delete(m.clients, client)
		close(client.Send())
		m.logger.Info("Client unregistered")
	}
}

func (m *Manager) findClientByUsername(username string) *connection.Client {
	for client := range m.clients {
		if client.GetUser() == username {
			return client
		}
	}
	return nil
}
