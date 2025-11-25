package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/zuczkows/room-chat/internal/chat"
	apperrors "github.com/zuczkows/room-chat/internal/errors"
	"github.com/zuczkows/room-chat/internal/middleware"
	"github.com/zuczkows/room-chat/internal/storage"
	"github.com/zuczkows/room-chat/internal/user"
)

type RegisterResponse struct {
	ID int64 `json:"id"`
}

type UserHandler struct {
	userService    *user.Service
	channelManager *chat.ChannelManager
	logger         *slog.Logger
	storage        *storage.MessageIndexer
}

func NewUserHandler(userService *user.Service, logger *slog.Logger, storage *storage.MessageIndexer, channelManager *chat.ChannelManager) *UserHandler {
	return &UserHandler{
		userService:    userService,
		channelManager: channelManager,
		logger:         logger,
		storage:        storage,
	}
}

func (u *UserHandler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req user.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		u.logger.Error("Failed to decode registration request", slog.Any("error", err))
		apperrors.SendError(w, http.StatusBadRequest, apperrors.InvalidJSON)
		return
	}
	if req.Username == "" {
		apperrors.SendError(w, http.StatusUnprocessableEntity, apperrors.UserNameEmpty)
		return
	}

	userProfileID, err := u.userService.Register(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, user.ErrUserOrNickAlreadyExists):
			apperrors.SendError(w, http.StatusConflict, apperrors.UsernameNickTaken)
		case errors.Is(err, user.ErrMissingRequiredFields):
			apperrors.SendError(w, http.StatusUnprocessableEntity, apperrors.MissingRequiredFields)
		default:
			u.logger.Error("Registration failed", slog.Any("error", err))
			apperrors.SendError(w, http.StatusInternalServerError, apperrors.InternalServer)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(RegisterResponse{ID: userProfileID})
}

func (u *UserHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	authenticatedUserID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		u.logger.Error("No authenticated user in context")
		apperrors.SendError(w, http.StatusUnauthorized, apperrors.AuthenticationRequired)
		return
	}

	var req user.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		u.logger.Error("Failed to decode update request", slog.Any("error", err))
		apperrors.SendError(w, http.StatusBadRequest, apperrors.InvalidJSON)
		return
	}

	updatedUser, err := u.userService.UpdateProfile(r.Context(), authenticatedUserID, req)
	if err != nil {
		u.logger.Error("Profile update failed", slog.Any("error", err))
		switch {
		case errors.Is(err, user.ErrNickAlreadyExists):
			apperrors.SendError(w, http.StatusConflict, user.ErrNickAlreadyExists.Error())
		default:
			apperrors.SendError(w, http.StatusInternalServerError, user.ErrInternalServer.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedUser)
}

func (u *UserHandler) HandleListMessages(w http.ResponseWriter, r *http.Request) {
	authenticatedUsername, ok := middleware.GetUsernameFromContext(r.Context())
	if !ok {
		u.logger.Error("No authenticated user in context")
		apperrors.SendError(w, http.StatusUnauthorized, apperrors.AuthenticationRequired)
		return
	}

	var req user.ListMessages
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		u.logger.Error("Failed to list messages request", slog.Any("error", err))
		apperrors.SendError(w, http.StatusBadRequest, apperrors.InvalidJSON)
		return
	}

	isUserAMember := u.channelManager.IsUserAMember(req.Channel, authenticatedUsername)
	if !isUserAMember {
		apperrors.SendError(w, http.StatusUnauthorized, apperrors.NotMemberOfChannel)
		return
	}
	msgs, _ := u.storage.ListDocuments(req.Channel)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(msgs); err != nil {
		u.logger.Error("failed to encode messages response", slog.Any("error", err))
	}
}
