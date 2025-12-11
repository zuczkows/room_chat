//go:build integration

package test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/zuczkows/room-chat/internal/elastic"
	"github.com/zuczkows/room-chat/internal/utils"
	"github.com/zuczkows/room-chat/test/internal/websocket"
)

type ErrorResponse struct {
	Error string `json:"error"`
}

func TestStorage(t *testing.T) {

	testUser1 := CreateTestUser1(t, users)
	testUser2 := CreateTestUser2(t, users)

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

	channelUser1 := "channel-user-1"
	_, err = wsUser1.Join(channelUser1)
	require.NoError(t, err)

	channelUser2 := "channel-user-2"
	_, err = wsUser2.Join(channelUser2)
	require.NoError(t, err)

	testMessageUser1 := "testMessageUser1"
	_, err = wsUser1.SendMessage(testMessageUser1, channelUser1)
	require.NoError(t, err)

	testMessageUser2 := "testMessageUser2"
	_, err = wsUser2.SendMessage(testMessageUser2, channelUser2)
	require.NoError(t, err)

	t.Run("user can get messages from channel he belongs to", func(t *testing.T) {
		request := prepareListMessagesRequestWithoutAuthHeader(t, channelUser1)
		request.Header.Set("Authorization", accessTokenUser1)
		time.Sleep(time.Second * 1)

		resp, err := http.DefaultClient.Do(request)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var messages []elastic.IndexedMessage
		err = json.NewDecoder(resp.Body).Decode(&messages)
		require.NoError(t, err)

		require.Equal(t, testMessageUser1, messages[0].Content)

	})

	t.Run("user can't get messages from channel he does not belongs to", func(t *testing.T) {
		request := prepareListMessagesRequestWithoutAuthHeader(t, channelUser2)
		request.Header.Set("Authorization", accessTokenUser1)

		resp, err := http.DefaultClient.Do(request)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

		var errResp ErrorResponse
		err = json.NewDecoder(resp.Body).Decode(&errResp)
		require.NoError(t, err)

		require.Equal(t, "You are not a member of this channel.", errResp.Error)
	})

	t.Run("user can't get messages with wrong token", func(t *testing.T) {
		request := prepareListMessagesRequestWithoutAuthHeader(t, channelUser1)
		request.Header.Set("Authorization", "Basic wrongcredentials")

		resp, err := http.DefaultClient.Do(request)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

		var errResp ErrorResponse
		err = json.NewDecoder(resp.Body).Decode(&errResp)
		require.NoError(t, err)

		require.Equal(t, "Authentication required.", errResp.Error)
	})

	t.Run("user can't get messages without authorization header", func(t *testing.T) {
		request := prepareListMessagesRequestWithoutAuthHeader(t, channelUser1)

		resp, err := http.DefaultClient.Do(request)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

		var errResp ErrorResponse
		err = json.NewDecoder(resp.Body).Decode(&errResp)
		require.NoError(t, err)

		require.Equal(t, "Authentication required.", errResp.Error)
	})

}

func prepareListMessagesRequestWithoutAuthHeader(t *testing.T, channelName string) *http.Request {
	request, err := http.NewRequest(http.MethodGet, "http://localhost:8080/channel/messages", nil)
	require.NoError(t, err)
	q := request.URL.Query()
	q.Add("channel", channelName)
	request.URL.RawQuery = q.Encode()
	return request
}
