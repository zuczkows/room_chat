package server

import (
	"context"
	"log/slog"
	"net/http"

	apperrors "github.com/zuczkows/room-chat/internal/errors"
	"github.com/zuczkows/room-chat/internal/protocol"
	"github.com/zuczkows/room-chat/internal/user"
)

type AuthMiddleware struct {
	users  *user.Users
	logger *slog.Logger
}

func NewAuthMiddleware(users *user.Users, logger *slog.Logger) *AuthMiddleware {
	return &AuthMiddleware{
		users:  users,
		logger: logger,
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

		profile, err := a.users.Login(r.Context(), username, password)
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
