package server

import (
	"context"
	"errors"
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
	"github.com/zuczkows/room-chat/internal/utils"
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

type Channels map[string]*chat.Channel
type Clients map[string]*connection.Client

type Manager struct {
	channels        Channels
	clients         Clients
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
		clients:         make(Clients),
		register:        make(chan *connection.Client),
		unregister:      make(chan *connection.Client),
		dispatchMessage: make(chan protocol.Message),
		logger:          logger,
		config:          cfg,
		userService:     userService,
	}
}

func (m *Manager) StartServ() {
	mux := http.NewServeMux()
	userHandler := handlers.NewUserHandler(m.userService, m.logger)
	authMiddleware := middleware.NewAuthMiddleware(m.userService, m.logger)

	mux.HandleFunc("GET /ws", m.ServeWS)
	mux.HandleFunc("POST /api/register", userHandler.HandleRegister)
	mux.Handle("PUT /api/profile", authMiddleware.BasicAuth(http.HandlerFunc(userHandler.HandleUpdate)))
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
	case protocol.LoginAction:
		m.handleLogin(message)
	}
}

func (m *Manager) handleLogin(message protocol.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()

	senderClient, exists := m.clients[message.ClientID]
	if !exists {
		m.logger.Error("Client not found for login", slog.String("clientID", message.ClientID))
		return
	}

	username, password, ok := utils.ParseBasicAuth(message.Token)
	if !ok {
		errorMsg := protocol.Message{
			Type:    protocol.ErrorMessage,
			Content: "missing or invalid credentials",
		}
		senderClient.Send() <- errorMsg
		return
	}
	userID, err := m.userService.Login(context.Background(), username, password)
	if err != nil {
		switch {
		case errors.Is(err, user.ErrUserNotFound):
			m.logger.Info("login attempt with wrong username",
				slog.String("username", username))
			errorMsg := protocol.Message{
				Type:    protocol.ErrorMessage,
				Content: "invalid username or password",
			}
			senderClient.Send() <- errorMsg
		case errors.Is(err, user.ErrInvalidPassword):
			m.logger.Info("login attempt with wrong password",
				slog.String("username", username))
			errorMsg := protocol.Message{
				Type:    protocol.ErrorMessage,
				Content: "invalid username or password",
			}
			senderClient.Send() <- errorMsg
		default:
			m.logger.Error("login internal service error",
				slog.String("username", username),
				slog.Any("error", err))
			errorMsg := protocol.Message{
				Type:    protocol.ErrorMessage,
				Content: "internal server error",
			}
			senderClient.Send() <- errorMsg
		}
		return
	}
	user, err := m.userService.GetUser(context.Background(), userID)
	if err != nil {
		errorMsg := protocol.Message{
			Type:    protocol.ErrorMessage,
			Content: "something went wrong",
		}
		senderClient.Send() <- errorMsg
		return
	}

	for _, channel := range m.channels {
		if channel.HasUser(username) {
			channel.AddClient(username, senderClient)
		}
	}

	senderClient.Authenticate(user.Username)
	loginMsg := protocol.Message{
		Type:    protocol.MessageActionSystem,
		Content: fmt.Sprintf("Welcome %s!", senderClient.GetUser()),
	}
	senderClient.Send() <- loginMsg

}

func (m *Manager) getAllClientsForUser(username string) []*connection.Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var userClients []*connection.Client
	for _, client := range m.clients {
		if client.IsAuthenticated() && client.GetUser() == username {
			userClients = append(userClients, client)
		}
	}
	return userClients
}

func (m *Manager) handleJoinChannel(message protocol.Message) {
	m.mu.Lock()

	channelName := message.Channel
	senderClient, exists := m.clients[message.ClientID]
	if !exists {
		m.logger.Error("Client not found for join channel", slog.String("clientID", message.ClientID))
		return
	}

	username := senderClient.GetUser()

	channel, exists := m.channels[channelName]
	if exists {
		if channel.HasUser(username) {
			userAlreadyInChannelMsg := protocol.Message{
				Type:    protocol.MessageActionSystem,
				Content: fmt.Sprintf("You are already in a channel: %s", channelName),
			}
			senderClient.Send() <- userAlreadyInChannelMsg
			return
		}
	} else {
		m.channels[channelName] = chat.NewChannel(channelName, m.logger)
		channel = m.channels[channelName]
		m.logger.Info("Created new channel", slog.String("channel", channelName))
	}
	m.mu.Unlock()

	userClients := m.getAllClientsForUser(username)
	for _, client := range userClients {
		channel.AddClient(username, client)
	}

	m.logger.Info("User joined channel",
		slog.String("user", message.User),
		slog.String("channel", channelName),
		slog.Int("connections", len(userClients)))

}

func (m *Manager) handleSendMessage(message protocol.Message) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	senderClient, exists := m.clients[message.ClientID]
	if !exists {
		m.logger.Error("Client not found for send message", slog.String("clientID", message.ClientID))
		return
	}

	channel, exists := m.channels[message.Channel]
	if exists {
		if channel.HasUser(senderClient.GetUser()) {
			// Note zuczkows - Maybe Broadcast should have user param in order to skip sending pushes to his own websocket
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
	senderClient, exists := m.clients[message.ClientID]
	if !exists {
		m.logger.Error("Client not found for leave channel", slog.String("clientID", message.ClientID))
		return
	}

	channel, exists := m.channels[channelName]
	if !exists {
		errorMsg := protocol.Message{
			Type:    protocol.MessageActionSystem,
			Content: fmt.Sprintf("channel %s does not exist", channelName),
		}
		senderClient.Send() <- errorMsg
		return
	}
	username := senderClient.GetUser()
	if channel.HasUser(username) {
		channel.RemoveClient(username, senderClient)

		if activeUsersCount := channel.ActiveUsersCount(); activeUsersCount == 0 {
			delete(m.channels, channelName)
			m.logger.Info("Channel deleted", slog.String("channel", channelName))
		}
		return
	}
	errorMsg := protocol.Message{
		Type:    protocol.MessageActionSystem,
		Content: fmt.Sprintf("You are not a member of channel '%s'", channelName),
	}
	senderClient.Send() <- errorMsg
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
	m.clients[client.ConnID] = client
	m.logger.Info("Client added", slog.Int("total_clients", len(m.clients)))
}

func (m *Manager) removeClient(client *connection.Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, exists := m.clients[client.ConnID]
	userName := client.GetUser()
	if exists {
		for _, channel := range m.channels {
			if channel.HasUser(userName) {
				channel.RemoveClient(userName, client)
			}
		}
		delete(m.clients, client.ConnID)
		close(client.Send())
		m.logger.Info("Client unregistered", slog.String("user", userName))
	}
}
