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
	"github.com/zuczkows/room-chat/internal/storage"
	"github.com/zuczkows/room-chat/internal/user"
	"github.com/zuczkows/room-chat/internal/utils"
)

const (
	ReadBufferSize  = 1024
	WriteBufferSize = 1024
)

const (
	MissingToken                = "Missing token."
	ClientNotFound              = "Client not found for login."
	MissingOrInvalidCredentials = "Missing or invalid credentials."
	InternalServerError         = "Internal sever error."
	AlreadyLoggedIn             = "Already logged in."
	AlreadyInChannel            = "You are already in this channel."
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

type Clients map[string]*connection.Client

type Server struct {
	clients         Clients
	userManager     *user.SessionManager
	channelManager  *chat.ChannelManager
	register        chan *connection.Client
	unregister      chan *connection.Client
	dispatchMessage chan protocol.Message
	mu              sync.RWMutex
	logger          *slog.Logger
	config          *config.Config
	server          *http.Server
	userService     *user.Service
	storage         *storage.MessageIndexer
}

func NewServer(logger *slog.Logger, cfg *config.Config, userService *user.Service, storage *storage.MessageIndexer, channelManager *chat.ChannelManager) *Server {
	return &Server{
		clients:         make(Clients),
		userManager:     user.NewSessionManager(logger),
		channelManager:  channelManager,
		register:        make(chan *connection.Client),
		unregister:      make(chan *connection.Client),
		dispatchMessage: make(chan protocol.Message),
		logger:          logger,
		config:          cfg,
		userService:     userService,
		storage:         storage,
	}
}

func (s *Server) Start() {
	mux := http.NewServeMux()
	userHandler := handlers.NewUserHandler(s.userService, s.logger, s.storage, s.channelManager)
	authMiddleware := middleware.NewAuthMiddleware(s.userService, s.logger)

	mux.HandleFunc("GET /ws", s.ServeWS)
	mux.HandleFunc("POST /api/register", userHandler.HandleRegister)
	mux.Handle("PUT /api/profile", authMiddleware.BasicAuth(http.HandlerFunc(userHandler.HandleUpdate)))
	mux.Handle("POST /channel/messages", authMiddleware.BasicAuth(http.HandlerFunc(userHandler.HandleListMessages)))
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

func (s *Server) handleMessage(message protocol.Message) {
	switch message.Action {
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
		s.logger.Error(ClientNotFound, slog.String("clientID", message.ClientID))
		return
	}
	if message.Request.Token == "" {
		s.sendError(senderClient, MissingToken, message.RequestID, protocol.AuthorizationError)
		return
	}

	username, password, ok := utils.ParseBasicAuth(message.Request.Token)
	if !ok {
		s.sendError(senderClient, MissingOrInvalidCredentials, message.RequestID, protocol.AuthorizationError)
		return
	}
	profile, err := s.userService.Login(context.Background(), username, password)
	if err != nil {
		switch {
		case errors.Is(err, user.ErrUserNotFound):
			s.logger.Info("login attempt with wrong username",
				slog.String("username", username))
			s.sendError(senderClient, "invalid username or password", message.RequestID, protocol.AuthorizationError)
		case errors.Is(err, user.ErrInvalidPassword):
			s.logger.Info("login attempt with wrong password",
				slog.String("username", username))
			s.sendError(senderClient, "invalid username or password", message.RequestID, protocol.AuthorizationError)
		default:
			s.logger.Error("login internal service error", slog.String("username", username), slog.Any("error", err))
			s.sendError(senderClient, InternalServerError, message.RequestID, protocol.InternalServerError)
		}
		return
	}

	if senderClient.IsAuthenticated() {
		s.sendError(senderClient, AlreadyLoggedIn, message.RequestID, protocol.ValidationError)
		return
	}
	senderClient.Authenticate(profile.Username)
	s.userManager.AddClientToUser(username, senderClient, profile)

	s.sendResponse(senderClient, fmt.Sprintf("Welcome %s!", profile.Username), message.RequestID)
}

func (s *Server) handleJoinChannel(message protocol.Message) {
	user, err := s.userManager.GetUser(message.User)
	if err != nil {
		s.logger.Error("User not found for join channel", slog.String("user", message.User))
		return
	}
	s.mu.Lock()
	senderClient, exists := s.clients[message.ClientID]
	if !exists {
		s.mu.Unlock()
		s.logger.Error("Client not found for login", slog.String("clientID", message.ClientID))
		return
	}
	channel := s.channelManager.GetOrCreate(message.Channel, s.logger, s.userManager, s.storage)

	err = s.channelManager.AddUser(message.Channel, user.Username())
	if err != nil {
		s.mu.Unlock()
		switch {
		case errors.Is(err, chat.ErrUserAlreadyExists):
			s.sendError(senderClient, AlreadyInChannel, message.RequestID, protocol.ConflictError)
		default:
			s.logger.Error("unexpected error adding user to channel", slog.String("user", user.Username()), slog.String("channel", message.Channel), slog.Any("error", err))
			s.sendError(senderClient, InternalServerError, message.RequestID, protocol.InternalServerError)
		}
		return
	}

	user.AddChannel(message.Channel)
	s.mu.Unlock()
	s.sendResponse(senderClient, fmt.Sprintf("Welcome in channel %s!", message.Channel), message.RequestID)
	channel.SendWelcomeMessage(user.Username())
}

func (s *Server) handleSendMessage(message protocol.Message) {
	s.mu.RLock()
	senderClient, exists := s.clients[message.ClientID]
	if !exists {
		s.mu.RUnlock()
		s.logger.Error("Client not found for send message", slog.String("clientID", message.ClientID))
		return
	}

	channel, err := s.channelManager.Get(message.Channel)
	s.mu.RUnlock()

	if err != nil {
		s.sendError(senderClient, fmt.Sprintf("Channel does not exist: %s", message.Channel), message.RequestID, "validation")
	}

	username := senderClient.GetUser()
	if !s.channelManager.IsUserAMember(message.Channel, username) {
		s.sendError(senderClient, fmt.Sprintf("You are not a member of this channel: %s", message.Channel), message.RequestID, "forbidden")
		return
	}
	s.sendResponse(senderClient, "message sent", message.RequestID)
	channel.SendMessage(message)
	s.logger.Info("Message sent to channel", slog.String("channel", message.Channel), slog.String("user", username))
	err = s.storage.IndexWSMessage(message)
	if err != nil {
		// NOTE(zuczkows): Perhaps some retry logic could be applied here - for now just log warning
		s.logger.Warn("failed to index message",
			slog.String("id", message.ID),
			slog.Any("error", err),
		)
	}
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

	channel, err := s.channelManager.Get(message.Channel)
	if err != nil {
		s.mu.RUnlock()
		s.sendError(senderClient, fmt.Sprintf("Channel %s does not exist", channelName), message.RequestID, protocol.ValidationError)
		return
	}
	s.mu.RUnlock()

	username := senderClient.GetUser()
	if !channel.HasUser(username) {
		s.sendError(senderClient, fmt.Sprintf("You are not a member of channel '%s'", channelName), message.RequestID, protocol.ForbiddenError)
		return
	}

	if err := channel.RemoveUser(username); err != nil {
		switch {
		case errors.Is(err, chat.ErrNotAMember):
			s.logger.Error("trying to remove non member from a channel", slog.String("user", username), slog.String("channel", channelName))
			return
		default:
			s.logger.Error("unexpected error while removing member", slog.String("user", username), slog.Any("error", err))
		}
	}

	user, err := s.userManager.GetUser(username)
	if err != nil {
		s.logger.Warn("lost synchronization - authorized user does not exists in userManagerl", slog.String("username", username))
	}
	user.RemoveChannel(channelName)

	s.sendResponse(senderClient, fmt.Sprintf("You left channel: %s!", message.Channel), message.RequestID)
	channel.SendLeaveMessage(user.Username())

	s.mu.Lock()
	if channel.ActiveUsersCount() == 0 {
		s.channelManager.Delete(channelName)
		s.logger.Info("Channel deleted", slog.String("channel", channelName))
	}
	s.mu.Unlock()

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

	user, err := s.userManager.GetUser(userName)
	if err != nil {
		s.logger.Warn("lost synchronization - user not found in userManager", slog.Any("error", err))
		return
	}
	s.userManager.RemoveClientFromUser(userName, client)

	if !user.HasConnections() {
		s.removeUserFromAllChannels(user)
	}

	s.logger.Info("Client unregistered", slog.String("user", userName))
}

func (s *Server) sendError(client *connection.Client, message, requestID string, errType protocol.ErrorType) {
	msg := protocol.Message{
		Type:      protocol.MessageTypeResponse,
		RequestID: requestID,
		Action:    protocol.ErrorMessage,
		Response: &protocol.Response{
			Success: false,
			RespErr: &protocol.ErrorDetails{
				Type:    errType,
				Message: message,
			},
		},
	}
	client.Send() <- msg
}

func (s *Server) sendResponse(client *connection.Client, message, requestID string) {
	msg := protocol.Message{
		Type:      protocol.MessageTypeResponse,
		RequestID: requestID,
		Action:    protocol.MessageActionText,
		Response: &protocol.Response{
			Content: message,
			Success: true,
		},
	}
	client.Send() <- msg
}

func (s *Server) removeUserFromAllChannels(user *user.User) {
	userName := user.Username()

	for _, channelName := range user.GetChannels() {
		s.mu.RLock()
		channel, err := s.channelManager.Get(channelName)
		if err != nil {
			s.logger.Warn("lost synchronization - user belongs to non existing channel", slog.String("username", userName), slog.String("channel", channelName))
			user.RemoveChannel(channelName)
			continue
		}
		if err := channel.RemoveUser(userName); err != nil {
			switch {
			case errors.Is(err, chat.ErrNotAMember):
				s.logger.Warn("trying to remove non member from a channel", slog.String("user", userName), slog.String("channel", channelName))
			default:
				s.logger.Error("unexpected error while removing member", slog.String("user", userName), slog.Any("error", err))
			}
			continue
		}
		user.RemoveChannel(channelName)
		s.mu.RUnlock()
		channel.SendLeaveMessage(userName)

		s.cleanupEmptyChannel(channelName, channel)

	}
}

func (s *Server) cleanupEmptyChannel(channelName string, channel *chat.Channel) {
	s.mu.Lock()
	if channel.ActiveUsersCount() == 0 {
		s.channelManager.Delete(channelName)
	}
	s.mu.Unlock()
}
