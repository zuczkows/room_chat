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

func TestTwoWebsocketConnections(t *testing.T) {

	testUser1 := CreateTestUser1(t, userService)
	testUser2 := CreateTestUser2(t, userService)

	wsUser1Connection1, err := websocket.NewRoomChatWS("localhost:8080", time.Second*10, "test-user-conn1")
	require.NoError(t, err)
	defer wsUser1Connection1.Close()

	wsUser1Connection2, err := websocket.NewRoomChatWS("localhost:8080", time.Second*10, "test-user-conn2")
	require.NoError(t, err)
	defer wsUser1Connection2.Close()

	wsUser2, err := websocket.NewRoomChatWS("localhost:8080", time.Second*10, "test-user-2")
	require.NoError(t, err)
	defer wsUser2.Close()

	accessTokenUser1 := utils.GetEncodedBase64Token(testUser1.Username, testUser1.Password)
	_, err = wsUser1Connection1.Login(accessTokenUser1)
	require.NoError(t, err)

	_, err = wsUser1Connection2.Login(accessTokenUser1)
	require.NoError(t, err)

	accessTokenUser2 := utils.GetEncodedBase64Token(testUser2.Username, testUser2.Password)
	_, err = wsUser2.Login(accessTokenUser2)
	require.NoError(t, err)
	channelUser2 := "channel-user-2"
	_, err = wsUser2.Join(channelUser2)
	require.NoError(t, err)

	t.Run("user websocket 1 joins a channel", func(t *testing.T) {
		joinChannelResponse, err := wsUser1Connection1.Join(channelUser2)
		require.NoError(t, err)

		require.Equal(t, fmt.Sprintf("Welcome in channel %s!", channelUser2), joinChannelResponse.Response.Content)
	})

	t.Run("two websocket connection receive message", func(t *testing.T) {
		testMessage := "Hello this is user-2"
		_, err := wsUser2.SendMessage(testMessage, channelUser2)
		require.NoError(t, err)

		require.Eventually(t,
			websocket.PushDelivered(wsUser1Connection1.WSClient, websocket.PushCriteria{
				Action:  protocol.MessageActionText,
				Content: testMessage,
			}),
			10*time.Second,
			2*time.Second,
		)

		require.Eventually(t,
			websocket.PushDelivered(wsUser1Connection2.WSClient, websocket.PushCriteria{
				Action:  protocol.MessageActionText,
				Content: testMessage,
			}),
			10*time.Second,
			2*time.Second,
		)
	})
}
