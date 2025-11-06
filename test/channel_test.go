//go:build integration2

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

func TestChannel(t *testing.T) {

	db, cleanup, err := SetupDB()
	require.NoError(t, err, "failed to setup database")
	defer cleanup()

	userService := SetupServer(db)

	testUser1 := CreateTestUser1(t, userService)
	testUser2 := CreateTestUser2(t, userService)

	wsUser1, err := websocket.NewRoomChatWS("localhost:8080", time.Second*10, "test-user")
	require.NoError(t, err)
	defer wsUser1.Close()

	wsUser2, err := websocket.NewRoomChatWS("localhost:8080", time.Second*10, "test-user")
	require.NoError(t, err)
	defer wsUser2.Close()

	accessTokenUser1 := utils.GetEncodedBase64Token(testUser1.Username, testUser1.Password)
	_, err = wsUser1.Login(accessTokenUser1)
	require.NoError(t, err)

	accessTokenUser2 := utils.GetEncodedBase64Token(testUser2.Username, testUser2.Password)
	_, err = wsUser2.Login(accessTokenUser2)
	require.NoError(t, err)
	channelUser2 := "channel-user-2"
	_, err = wsUser2.Join(channelUser2)
	require.NoError(t, err)

	t.Run("joining a channel", func(t *testing.T) {
		channelName := "General"
		joinChannelResponse, err := wsUser1.Join(channelName)
		require.NoError(t, err)

		assert.Equal(t, fmt.Sprintf("Welcome in channel %s!", channelName), joinChannelResponse.Response.Content)
	})

	t.Run("sent message to non existing channel", func(t *testing.T) {
		channelName := "Group-2"
		_, err := wsUser1.SendMessage("message", channelName)
		require.Error(t, err)

		var wsErr *websocket.WSError
		if errors.As(err, &wsErr) {
			require.Equal(t, fmt.Sprintf("Channel does not exist: %s", channelName), wsErr.Message)
			require.Equal(t, "validation", wsErr.Type)
		} else {
			t.Fatal("unexpected websocket error:", err)
		}
	})

	t.Run("sent message to a channel that user dont belong to", func(t *testing.T) {
		_, err := wsUser1.SendMessage("message", channelUser2)
		require.Error(t, err)

		var wsErr *websocket.WSError
		if errors.As(err, &wsErr) {
			require.Equal(t, fmt.Sprintf("You are not a member of this channel: %s", channelUser2), wsErr.Message)
			require.Equal(t, "forbidden", wsErr.Type)
		} else {
			t.Fatal("unexpected websocket error:", err)
		}
	})

}
