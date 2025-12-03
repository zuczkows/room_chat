package test

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zuczkows/room-chat/internal/protocol"
	"github.com/zuczkows/room-chat/internal/user"
)

func CreateTestUser1(t *testing.T, userService *user.Service) *protocol.CreateUserRequest {
	randomNum := rand.Intn(10000) + 1
	testUser := &protocol.CreateUserRequest{
		Username: fmt.Sprintf("test-user-1-%d", randomNum),
		Nick:     fmt.Sprintf("test-nick-%d", randomNum),
		Password: "password2137!",
	}

	_, err := userService.Register(context.Background(), *testUser)
	require.NoError(t, err, "failed to create test user")

	return testUser
}

// NOTE(zuczkows): Could be more general - leaving for now (rule of three)
func CreateTestUser2(t *testing.T, userService *user.Service) *protocol.CreateUserRequest {
	randomNum := rand.Intn(10000) + 1
	testUser := &protocol.CreateUserRequest{
		Username: fmt.Sprintf("test-user-2-%d", randomNum),
		Nick:     fmt.Sprintf("test-nick-%d", randomNum),
		Password: "password2137!",
	}

	_, err := userService.Register(context.Background(), *testUser)
	require.NoError(t, err, "failed to create test user")

	return testUser
}
