package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/zuczkows/room-chat/internal/channels"
	"github.com/zuczkows/room-chat/internal/elastic"
	apperrors "github.com/zuczkows/room-chat/internal/errors"
	"github.com/zuczkows/room-chat/internal/protocol"
	"github.com/zuczkows/room-chat/internal/user"
)

type UserHandler struct {
	users    *user.Users
	channels *channels.Channels
	logger   *slog.Logger
	elastic  *elastic.MessageIndexer
}

func NewUserHandler(users *user.Users, logger *slog.Logger, elastic *elastic.MessageIndexer, channels *channels.Channels) *UserHandler {
	return &UserHandler{
		users:    users,
		channels: channels,
		logger:   logger,
		elastic:  elastic,
	}
}

func (u *UserHandler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req protocol.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		u.logger.Error("Failed to decode registration request", slog.Any("error", err))
		apperrors.SendError(w, http.StatusBadRequest, protocol.InvalidJSON)
		return
	}
	if req.Username == "" {
		apperrors.SendError(w, http.StatusUnprocessableEntity, protocol.UserNameEmpty)
		return
	}
	if req.Password == "" {
		apperrors.SendError(w, http.StatusUnprocessableEntity, protocol.PasswordEmpty)
		return
	}

	userProfileID, err := u.users.Register(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, user.ErrUserOrNickAlreadyExists):
			apperrors.SendError(w, http.StatusConflict, protocol.UsernameNickTaken)
		case errors.Is(err, user.ErrMissingRequiredFields):
			apperrors.SendError(w, http.StatusUnprocessableEntity, protocol.MissingRequiredFields)
		default:
			u.logger.Error("Registration failed", slog.Any("error", err))
			apperrors.SendError(w, http.StatusInternalServerError, protocol.InternalServer)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(protocol.RegisterResponse{ID: userProfileID})
}

func (u *UserHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	authenticatedUserID, err := GetUserIDFromContext(r.Context())
	if err != nil {
		u.logger.Error("No authenticated user in context")
		apperrors.SendError(w, http.StatusInternalServerError, protocol.InternalServer)
		return
	}

	var req protocol.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		u.logger.Error("Failed to decode update request", slog.Any("error", err))
		apperrors.SendError(w, http.StatusBadRequest, protocol.InvalidJSON)
		return
	}

	updatedUser, err := u.users.UpdateProfile(r.Context(), authenticatedUserID, req)
	if err != nil {
		u.logger.Error("Profile update failed", slog.Any("error", err))
		switch {
		case errors.Is(err, user.ErrNickAlreadyExists):
			apperrors.SendError(w, http.StatusConflict, protocol.NickAlreadyExists)
		default:
			apperrors.SendError(w, http.StatusInternalServerError, protocol.InternalServer)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedUser)
}

func (u *UserHandler) HandleListMessages(w http.ResponseWriter, r *http.Request) {
	authenticatedUsername, err := GetUsernameFromContext(r.Context())
	if err != nil {
		u.logger.Error("No authenticated user in context")
		apperrors.SendError(w, http.StatusInternalServerError, protocol.InternalServer)
		return
	}

	channel := r.URL.Query().Get("channel")
	if channel == "" {
		apperrors.SendError(w, http.StatusBadRequest, protocol.MissingRequiredFields)
		return
	}

	isUserAMember, err := u.channels.IsUserAMember(channel, authenticatedUsername)
	if err != nil {
		switch {
		case errors.Is(err, channels.ErrChannelDoesNotExist):
			// do not inform about not existing channel - information disclosure
			apperrors.SendError(w, http.StatusBadRequest, protocol.NotMemberOfChannel)
			return
		default:
			apperrors.SendError(w, http.StatusInternalServerError, protocol.InternalServer)
			return
		}
	}
	if !isUserAMember {
		apperrors.SendError(w, http.StatusUnauthorized, protocol.NotMemberOfChannel)
		return
	}
	msgs, err := u.elastic.ListDocuments(channel)
	if err != nil {
		u.logger.Error("Fetching document from ES failed", slog.Any("error", err))
		apperrors.SendError(w, http.StatusInternalServerError, protocol.InternalServer)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(msgs); err != nil {
		u.logger.Error("failed to encode messages response", slog.Any("error", err))
	}
}
