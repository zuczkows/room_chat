package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zuczkows/room-chat/internal/user"
)

func CreateTestUser1(t *testing.T, userService *user.Service) *user.CreateUserRequest {
	testUser := &user.CreateUserRequest{
		Username: "test-user-1",
		Nick:     "test-nick",
		Password: "password2137!",
	}

	_, err := userService.Register(context.Background(), *testUser)
	require.NoError(t, err, "failed to create test user")

	return testUser
}

// NOTE(zuczkows): Could be more general - leaving for now (rule of three)
func CreateTestUser2(t *testing.T, userService *user.Service) *user.CreateUserRequest {
	testUser := &user.CreateUserRequest{
		Username: "test-user-2",
		Nick:     "test-nick-2",
		Password: "password2137!",
	}

	_, err := userService.Register(context.Background(), *testUser)
	require.NoError(t, err, "failed to create test user")

	return testUser
}
