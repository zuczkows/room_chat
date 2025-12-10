package server

import (
	"context"
	"errors"
)

type contextKey string

const (
	userIDKey   contextKey = "userID"
	usernameKey contextKey = "username"
)

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
