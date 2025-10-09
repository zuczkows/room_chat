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

type Server struct {
	channels        Channels
	clients         Clients
	userManager     *user.UserManager
	register        chan *connection.Client
	unregister      chan *connection.Client
	dispatchMessage chan protocol.Message
	mu              sync.RWMutex
	logger          *slog.Logger
	config          *config.Config
	server          *http.Server
	userService     *user.Service
}

func NewServer(logger *slog.Logger, cfg *config.Config, userService *user.Service) *Server {
	return &Server{
		channels:        make(map[string]*chat.Channel),
		clients:         make(Clients),
		userManager:     user.NewUserManager(logger),
		register:        make(chan *connection.Client),
		unregister:      make(chan *connection.Client),
		dispatchMessage: make(chan protocol.Message),
		logger:          logger,
		config:          cfg,
		userService:     userService,
	}
}

func (s *Server) StartServ() {
	mux := http.NewServeMux()
	userHandler := handlers.NewUserHandler(s.userService, s.logger)
	authMiddleware := middleware.NewAuthMiddleware(s.userService, s.logger)

	mux.HandleFunc("GET /ws", s.ServeWS)
	mux.HandleFunc("POST /api/register", userHandler.HandleRegister)
	mux.Handle("PUT /api/profile", authMiddleware.BasicAuth(http.HandlerFunc(userHandler.HandleUpdate)))
	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Server.Port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	s.logger.Info("Starting server", "address", s.config.Server.Port)
	log.Fatal(s.server.ListenAndServe())
}

func (s *Server) Run() {
	for {
		select {
		case client := <-s.register:
			s.addClient(client)
		case client := <-s.unregister:
			s.removeClient(client)
		case message := <-s.dispatchMessage:
			s.handleMessage(message)
		}

	}
}

func (s *Server) handleMessage(message protocol.Message) {
	switch message.Type {
	case protocol.MesageActionJoin:
		s.handleJoinChannel(message)
	case protocol.MessageActionLeave:
		s.handleLeaveChannel(message)
	case protocol.MessageActionText:
		s.handleSendMessage(message)
	case protocol.LoginAction:
		s.handleLogin(message)
	}
}

func (s *Server) handleLogin(message protocol.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()

	senderClient, exists := s.clients[message.ClientID]
	if !exists {
		s.logger.Error("Client not found for login", slog.String("clientID", message.ClientID))
		return
	}

	username, password, ok := utils.ParseBasicAuth(message.Token)
	if !ok {
		s.sendMessageToClient(senderClient, protocol.ErrorMessage, "missing or invalid credentials")
		return
	}
	userID, err := s.userService.Login(context.Background(), username, password)
	if err != nil {
		switch {
		case errors.Is(err, user.ErrUserNotFound):
			s.logger.Info("login attempt with wrong username",
				slog.String("username", username))
			s.sendMessageToClient(senderClient, protocol.ErrorMessage, "invalid username or password")
		case errors.Is(err, user.ErrInvalidPassword):
			s.logger.Info("login attempt with wrong password",
				slog.String("username", username))
			s.sendMessageToClient(senderClient, protocol.ErrorMessage, "invalid username or password")
		default:
			s.logger.Error("login internal service error",
				slog.String("username", username),
				slog.Any("error", err))
			s.sendMessageToClient(senderClient, protocol.ErrorMessage, "internal server error")
		}
		return
	}
	profile, err := s.userService.GetProfile(context.Background(), userID)
	if err != nil {
		s.sendMessageToClient(senderClient, protocol.ErrorMessage, "internal server error")
		s.logger.Error("failed to fetch user profile", slog.Int64("user_id", userID), slog.String("username", username))
		return
	}

	senderClient.Authenticate(profile.Username)
	s.userManager.AddConnectionToUser(username, senderClient, profile)

	s.sendMessageToClient(senderClient, protocol.MessageActionSystem, fmt.Sprintf("Welcome %s!", senderClient.GetUser()))
}

func (s *Server) handleJoinChannel(message protocol.Message) {
	user, exists := s.userManager.GetUser(message.User)
	if !exists {
		s.logger.Error("User not found for join channel", slog.String("user", message.User))
		return
	}
	s.mu.Lock()
	senderClient, exists := s.clients[message.ClientID]
	if !exists {
		s.logger.Error("Client not found for login", slog.String("clientID", message.ClientID))
		return
	}
	channel, exists := s.channels[message.Channel]
	if !exists {
		s.channels[message.Channel] = chat.NewChannel(message.Channel, s.logger)
		channel = s.channels[message.Channel]
		s.logger.Info("Created new channel", slog.String("channel", message.Channel))
	}
	s.mu.Unlock()

	err := channel.AddUser(user.Username())
	if err != nil {
		switch {
		case errors.Is(err, chat.ErrUserAlreadyExists):
			s.sendMessageToClient(senderClient, protocol.ErrorMessage, "You are already in this channel")
		default:
			s.logger.Error("unexpected error adding user to channel",
				slog.String("user", user.Username()),
				slog.String("channel", message.Channel),
				slog.Any("error", err))
			s.sendMessageToClient(senderClient, protocol.ErrorMessage, "Internal server error")
		}
		return
	}
	joinMsg := protocol.Message{
		Type:    protocol.MessageActionSystem,
		Channel: message.Channel,
		Content: fmt.Sprintf("%s joined the channel", user.Username()),
	}
	s.broadcastToChannel(message.Channel, joinMsg)
}

func (s *Server) broadcastToChannel(channelName string, message protocol.Message) {
	s.mu.RLock()
	channel, exists := s.channels[channelName]
	s.mu.RUnlock()

	if !exists {
		s.logger.Error("error while broadcasting to channel", slog.String("channel doesnt exists", channelName))
		return
	}

	users := channel.GetUsers()
	for _, username := range users {
		if user, exists := s.userManager.GetUser(username); exists {
			user.SendEvent(message)
		}
	}
}

func (s *Server) handleSendMessage(message protocol.Message) {
	s.mu.RLock()
	senderClient, exists := s.clients[message.ClientID]
	if !exists {
		s.mu.RUnlock()
		s.logger.Error("Client not found for send message", slog.String("clientID", message.ClientID))
		return
	}

	channel, exists := s.channels[message.Channel]
	s.mu.RUnlock()

	if !exists {
		s.sendMessageToClient(senderClient, protocol.MessageActionSystem,
			fmt.Sprintf("Channel does not exist: %s", message.Channel))
		return
	}

	username := senderClient.GetUser()
	if !channel.HasUser(username) {
		s.sendMessageToClient(senderClient, protocol.MessageActionSystem,
			fmt.Sprintf("You are not a member of this channel: %s", message.Channel))
		return
	}

	s.broadcastToChannel(message.Channel, message)
	s.logger.Info("Message sent to channel",
		slog.String("channel", message.Channel),
		slog.String("user", username))
}

func (s *Server) handleLeaveChannel(message protocol.Message) {
	s.mu.RLock()
	channelName := message.Channel
	senderClient, exists := s.clients[message.ClientID]
	if !exists {
		s.mu.RUnlock()
		s.logger.Error("Client not found for leave channel", slog.String("clientID", message.ClientID))
		return
	}

	channel, exists := s.channels[channelName]
	if !exists {
		s.mu.RUnlock()
		s.sendMessageToClient(senderClient, protocol.ErrorMessage, fmt.Sprintf("channel %s does not exist", channelName))
		return
	}
	s.mu.RUnlock()

	username := senderClient.GetUser()
	if !channel.HasUser(username) {
		s.sendMessageToClient(senderClient, protocol.ErrorMessage, fmt.Sprintf("You are not a member of channel '%s'", channelName))
		return
	}

	if err := channel.RemoveUser(username); err != nil {
		switch {
		case errors.Is(err, chat.ErrNotAMember):
			s.logger.Error("trying to remove non member from a channel",
				slog.String("user", username),
				slog.String("channel", channelName))
			return
		default:
			s.logger.Error("unexpected error while removing member",
				slog.String("user", username),
				slog.Any("error", err))
		}
	}
	leaveMsg := protocol.Message{
		Type:    protocol.MessageActionSystem,
		Channel: channelName,
		Content: fmt.Sprintf("%s left the channel", username),
	}
	s.broadcastToChannel(channelName, leaveMsg)

	s.mu.Lock()
	if channel.ActiveUsersCount() == 0 {
		delete(s.channels, channelName)
		s.logger.Info("Channel deleted", slog.String("channel", channelName))
	}
	s.mu.Unlock()

}

func (s *Server) ServeWS(w http.ResponseWriter, r *http.Request) {
	upgrader := getUpgraderWithConfig(s.config)
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", slog.Any("error", err))
		return
	}

	client := connection.NewClient(conn, s.unregister, s.dispatchMessage, s.logger)

	s.register <- client
	go client.ReadMessages()
	go client.WriteMessages()

}

func (s *Server) addClient(client *connection.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[client.ConnID] = client
	s.logger.Info("Client added", slog.Int("total_clients", len(s.clients)))
}

func (s *Server) removeClient(client *connection.Client) {
	s.mu.Lock()
	userName := client.GetUser()
	_, exists := s.clients[client.ConnID]
	if exists {
		delete(s.clients, client.ConnID)
		close(client.Send())
	}
	s.mu.Unlock()

	s.userManager.RemoveConnectionFromUser(userName, client)
	if user, exists := s.userManager.GetUser(userName); !exists || !user.HasConnections() {
		s.removeUserFromAllChannels(userName)
	}

	s.logger.Info("Client unregistered", slog.String("user", userName))
}

func (s *Server) sendMessageToClient(client *connection.Client, messageAction protocol.MessageAction, message string) {
	msg := protocol.Message{
		Type:    messageAction,
		Content: message,
	}
	client.Send() <- msg
}

func (s *Server) removeUserFromAllChannels(userName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for channelName, channel := range s.channels {
		if channel.HasUser(userName) {
			if err := channel.RemoveUser(userName); err != nil {
				switch {
				case errors.Is(err, chat.ErrNotAMember):
					s.logger.Error("trying to remove non member from a channel",
						slog.String("user", userName),
						slog.String("channel", channelName))
					return
				default:
					s.logger.Error("unexpected error while removing member",
						slog.String("user", userName),
						slog.Any("error", err))
				}
			}
			leaveMsg := protocol.Message{
				Type:    protocol.MessageActionSystem,
				Channel: channelName,
				Content: fmt.Sprintf("%s left the channel", userName),
			}
			s.mu.Unlock()
			s.broadcastToChannel(channelName, leaveMsg)
			s.mu.Lock()

			if channel.ActiveUsersCount() == 0 {
				delete(s.channels, channelName)
				s.logger.Info("Channel deleted", slog.String("channel", channelName))
			}
		}
	}
}
