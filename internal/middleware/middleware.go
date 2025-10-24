package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/zuczkows/room-chat/internal/user"
)

type contextKey string

const UserContextKey contextKey = "user"

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

		userID, err := a.userService.Login(r.Context(), username, password)
		if err != nil {
			a.logger.Debug("Authentication failed", slog.String("username", username))
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), UserContextKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetUserIDFromContext(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(UserContextKey).(int64)
	return userID, ok
}
