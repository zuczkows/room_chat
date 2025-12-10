//go:build integration

package test

import (
	"fmt"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/zuczkows/room-chat/internal/protocol"
	"github.com/zuczkows/room-chat/internal/utils"
	"github.com/zuczkows/room-chat/test/internal/websocket"
)

func TestChannel(t *testing.T) {

	testUser1 := CreateTestUser1(t, users)
	testUser2 := CreateTestUser2(t, users)

	wsUser1, err := websocket.NewRoomChatWS("localhost:8080", time.Second*10, "test-user")
	require.NoError(t, err)
	defer wsUser1.Close()

	wsUser2, err := websocket.NewRoomChatWS("localhost:8080", time.Second*10, "test-user-2")
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

	t.Run("join a channel", func(t *testing.T) {
		channelName := "General"
		joinChannelResponse, err := wsUser1.Join(channelName)
		require.NoError(t, err)

		require.Equal(t, fmt.Sprintf("Welcome in channel %s!", channelName), joinChannelResponse.Response.Content)
	})

	t.Run("sent message to non existing channel", func(t *testing.T) {
		channelName := "Group-2"
		_, err := wsUser1.SendMessage("message", channelName)
		require.Error(t, err)

		var wsErr *websocket.WSError
		require.ErrorAs(t, err, &wsErr)
		require.Equal(t, fmt.Sprintf("You are not a member of this channel: %s", channelName), wsErr.Message)
		require.Equal(t, protocol.ValidationError, wsErr.Type)
	})

	t.Run("sent message to a channel that user doesn't belong to", func(t *testing.T) {
		_, err := wsUser1.SendMessage("message", channelUser2)
		require.Error(t, err)

		var wsErr *websocket.WSError
		require.ErrorAs(t, err, &wsErr)
		require.Equal(t, fmt.Sprintf("You are not a member of this channel: %s", channelUser2), wsErr.Message)
		require.Equal(t, protocol.ForbiddenError, wsErr.Type)
	})

	t.Run("user got a push when someone joins a channel", func(t *testing.T) {
		_, err := wsUser1.Join(channelUser2)
		require.NoError(t, err)

		require.Eventually(t,
			websocket.PushDelivered(wsUser2.WSClient, websocket.PushCriteria{
				Action:  protocol.MessageActionSystem,
				Content: fmt.Sprintf("%s joined the channel", testUser1.Username),
			}),
			10*time.Second,
			2*time.Second,
		)
	})

	t.Run("user in a channel receive message", func(t *testing.T) {
		testMessage := "Hello this is user1"
		_, err := wsUser1.SendMessage(testMessage, channelUser2)
		require.NoError(t, err)

		require.Eventually(t,
			websocket.PushDelivered(wsUser2.WSClient, websocket.PushCriteria{
				Action:  protocol.MessageActionText,
				Content: testMessage,
			}),
			10*time.Second,
			2*time.Second,
		)
	})

	t.Run("leave a channel", func(t *testing.T) {
		leftChannelResponse, err := wsUser2.Leave(channelUser2)
		require.NoError(t, err)

		require.Equal(t, fmt.Sprintf("You left channel: %s!", channelUser2), leftChannelResponse.Response.Content)
	})

	t.Run("user got notificaiton that someone left a channel", func(t *testing.T) {
		testMessage := fmt.Sprintf("%s left the channel", testUser2.Username)
		require.Eventually(t,
			websocket.PushDelivered(wsUser1.WSClient, websocket.PushCriteria{
				Action:  protocol.MessageActionSystem,
				Content: testMessage,
			}),
			10*time.Second,
			2*time.Second,
		)
	})

	t.Run("user cant send messages to a channel he left", func(t *testing.T) {
		_, err := wsUser2.SendMessage("message", channelUser2)
		require.Error(t, err)

		var wsErr *websocket.WSError
		require.ErrorAs(t, err, &wsErr)
		require.Equal(t, fmt.Sprintf("You are not a member of this channel: %s", channelUser2), wsErr.Message)
		require.Equal(t, protocol.ForbiddenError, wsErr.Type)
	})

	t.Run("user who is not in a channel doesnt receive a message", func(t *testing.T) {
		wsUser2.ClearPushes()

		testMessage := "Hello this is again user1"
		_, err := wsUser1.SendMessage(testMessage, channelUser2)
		require.NoError(t, err)

		pushes := wsUser2.GetPushes(protocol.MessageActionText)
		require.Equal(t, 0, len(pushes))
	})

}
