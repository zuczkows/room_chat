//go:build integration

package test

import (
	"fmt"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/zuczkows/room-chat/internal/protocol"
	"github.com/zuczkows/room-chat/internal/server"
	"github.com/zuczkows/room-chat/internal/utils"
	"github.com/zuczkows/room-chat/test/internal/websocket"
)

func TestLogin(t *testing.T) {
	testUser := CreateTestUser1(t, users)

	t.Run("successful_login", func(t *testing.T) {
		ws, err := websocket.NewRoomChatWS("localhost:8080", time.Second*10, "test-user")
		require.NoError(t, err)
		defer ws.Close()

		accessToken := utils.GetEncodedBase64Token(testUser.Username, testUser.Password)
		loginResponse, err := ws.Login(accessToken)
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("Welcome %s!", testUser.Username), loginResponse.Response.Content)
	})

	t.Run("duplicate_login_error", func(t *testing.T) {
		ws, err := websocket.NewRoomChatWS("localhost:8080", time.Second*10, "test-user")
		require.NoError(t, err)
		defer ws.Close()

		accessToken := utils.GetEncodedBase64Token(testUser.Username, testUser.Password)

		_, err = ws.Login(accessToken)
		require.NoError(t, err)

		_, err = ws.Login(accessToken)
		require.Error(t, err)

		var wsErr *websocket.WSError
		require.ErrorAs(t, err, &wsErr)
		require.Equal(t, server.AlreadyLoggedIn, wsErr.Message)
		require.Equal(t, protocol.ValidationError, wsErr.Type)
	})

	t.Run("wrong credentials", func(t *testing.T) {
		ws, err := websocket.NewRoomChatWS("localhost:8080", time.Second*10, "test-user")
		require.NoError(t, err)
		defer ws.Close()

		accessToken := utils.GetEncodedBase64Token(testUser.Username, "wrong-password")

		_, err = ws.Login(accessToken)
		require.Error(t, err)

		var wsErr *websocket.WSError

		require.ErrorAs(t, err, &wsErr)
		require.Equal(t, "invalid username or password", wsErr.Message)
		require.Equal(t, protocol.AuthorizationError, wsErr.Type)
	})

}
