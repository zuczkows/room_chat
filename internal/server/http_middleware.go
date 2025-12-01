package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	apperrors "github.com/zuczkows/room-chat/internal/errors"
	"github.com/zuczkows/room-chat/internal/protocol"
	"github.com/zuczkows/room-chat/internal/user"
)

type contextKey string

const (
	userIDKey   contextKey = "userID"
	usernameKey contextKey = "username"
)

type AuthMiddleware struct {
	userService *user.Service
	logger      *slog.Logger
}

func NewAuthMiddleware(userService *user.Service, logger *slog.Logger) *AuthMiddleware {
	return &AuthMiddleware{
		userService: userService,
		logger:      logger,
	}
}

func (a *AuthMiddleware) BasicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			a.logger.Debug("Missing or invalid Authorization header")
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			apperrors.SendError(w, http.StatusUnauthorized, protocol.AuthenticationRequired)
			return
		}

		profile, err := a.userService.Login(r.Context(), username, password)
		if err != nil {
			a.logger.Debug("Authentication failed", slog.String("username", username))
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			apperrors.SendError(w, http.StatusUnauthorized, protocol.InvalidCredentials)
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey, profile.ID)
		ctx = context.WithValue(ctx, usernameKey, username)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetUserIDFromContext(ctx context.Context) (int64, error) {
	userID, ok := ctx.Value(userIDKey).(int64)
	if !ok {
		return 0, errors.New("no authenticated user in context")
	}
	return userID, nil
}

func GetUsernameFromContext(ctx context.Context) (string, error) {
	username, ok := ctx.Value(usernameKey).(string)
	if !ok {
		return "", errors.New("no authenticated user in context")
	}
	return username, nil
}
