package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/zuczkows/room-chat/internal/user"
)

type contextKey string

const (
	UserIDKey   contextKey = "userID"
	UsernameKey contextKey = "username"
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
			http.Error(w, "Authorization required", http.StatusUnauthorized)
			return
		}

		profile, err := a.userService.Login(r.Context(), username, password)
		if err != nil {
			a.logger.Debug("Authentication failed", slog.String("username", username))
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), UserIDKey, profile.ID)
		ctx = context.WithValue(ctx, UsernameKey, username)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetUserIDFromContext(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(UserIDKey).(int64)
	return userID, ok
}

func GetUsernameFromContext(ctx context.Context) (string, bool) {
	username, ok := ctx.Value(UsernameKey).(string)
	return username, ok
}
