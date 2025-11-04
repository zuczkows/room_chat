package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/zuczkows/room-chat/internal/middleware"
	"github.com/zuczkows/room-chat/internal/user"
)

var (
	ErrUsernameNickTaken     = errors.New("username or nickname is already taken")
	ErrInternalServer        = errors.New("something went wrong on our side")
	ErrMissingRequiredFields = errors.New("some required fields are missing")
	ErrUseNameEmpty          = errors.New("username can not be empty")
)

type RegisterResponse struct {
	ID int64 `json:"id"`
}

type UserHandler struct {
	userService *user.Service
	logger      *slog.Logger
}

func NewUserHandler(userService *user.Service, logger *slog.Logger) *UserHandler {
	return &UserHandler{
		userService: userService,
		logger:      logger,
	}
}

func (u *UserHandler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req user.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		u.logger.Error("Failed to decode registration request", slog.Any("error", err))
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	userProfileID, err := u.userService.Register(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, user.ErrUserOrNickAlreadyExists):
			http.Error(w, ErrUsernameNickTaken.Error(), http.StatusConflict)
		case errors.Is(err, user.ErrMissingRequiredFields):
			http.Error(w, ErrMissingRequiredFields.Error(), http.StatusUnprocessableEntity)
		case errors.Is(err, user.ErrUserEmpty):
			http.Error(w, ErrUseNameEmpty.Error(), http.StatusUnprocessableEntity)
		default:
			u.logger.Error("Registration failed", slog.Any("error", err))
			http.Error(w, ErrInternalServer.Error(), http.StatusInternalServerError)
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
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	var req user.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		u.logger.Error("Failed to decode update request", slog.Any("error", err))
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	updatedUser, err := u.userService.UpdateProfile(r.Context(), authenticatedUserID, req)
	if err != nil {
		u.logger.Error("Profile update failed", slog.Any("error", err))
		switch {
		case errors.Is(err, user.ErrNickAlreadyExists):
			http.Error(w, user.ErrNickAlreadyExists.Error(), http.StatusConflict)
		default:
			http.Error(w, user.ErrInternalServer.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedUser)
}
