//go:build integration

package test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zuczkows/room-chat/internal/utils"
	"github.com/zuczkows/room-chat/test/internal/websocket"
)

func TestLogin(t *testing.T) {

	db, cleanup, err := SetupDB()
	require.NoError(t, err, "failed to setup database")
	defer cleanup()

	userService := SetupServer(db)

	testUser := CreateTestUser1(t, userService)

	t.Run("successful_login", func(t *testing.T) {
		ws, err := websocket.NewRoomChatWS("localhost:8080", time.Second*10, "test-user")
		require.NoError(t, err)
		defer ws.Close()

		accessToken := utils.GetEncodedBase64Token(testUser.Username, testUser.Password)
		loginResponse, err := ws.Login(accessToken)
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("Welcome %s!", testUser.Username), loginResponse.Response.Content)
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
		if errors.As(err, &wsErr) {
			require.Equal(t, "Already logged in", wsErr.Message)
			require.Equal(t, "validation", wsErr.Type)
		} else {
			t.Fatal("unexpected websocket error:", err)
		}
	})

	t.Run("wrong credentials", func(t *testing.T) {
		ws, err := websocket.NewRoomChatWS("localhost:8080", time.Second*10, "test-user")
		require.NoError(t, err)
		defer ws.Close()

		accessToken := utils.GetEncodedBase64Token(testUser.Username, "wrong-password")

		_, err = ws.Login(accessToken)
		require.Error(t, err)

		var wsErr *websocket.WSError
		if errors.As(err, &wsErr) {
			require.Equal(t, "invalid username or password", wsErr.Message)
			require.Equal(t, "authorization", wsErr.Type)
		} else {
			t.Fatal("unexpected websocket error:", err)
		}

	})

}
