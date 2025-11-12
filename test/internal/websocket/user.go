package websocket

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zuczkows/room-chat/internal/protocol"
)

type RoomChatWS struct {
	*WSClient
}

func NewRoomChatWS(baseURL string, wsTimeout time.Duration, user string) (*RoomChatWS, error) {
	baseURL = strings.TrimPrefix(baseURL, "https://")
	wsURL := fmt.Sprintf("ws://%s/ws", baseURL)

	wsClient, err := NewWSClient(wsURL, wsTimeout, user)
	if err != nil {
		return nil, err
	}

	return &RoomChatWS{
		WSClient: wsClient,
	}, nil
}

func (a *RoomChatWS) sendRequestAndProcessResponse(request *protocol.Message) (*protocol.Message, error) {
	response, err := a.SendRequest(request)
	if err != nil {
		return nil, err
	}
	if !response.Response.Success {
		return nil, HandleWSError(response)
	}

	resBytes, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}

	resPayload := &protocol.Message{}
	if err := json.Unmarshal(resBytes, resPayload); err != nil {
		return nil, err
	}

	return resPayload, nil
}

func (a *RoomChatWS) Login(accessToken string) (*protocol.Message, error) {
	request := &protocol.Message{
		Action: protocol.MessageAction("login"),
		Request: protocol.Request{
			Token: accessToken,
		},
	}
	return a.sendRequestAndProcessResponse(request)
}

func (a *RoomChatWS) Join(channel string) (*protocol.Message, error) {
	request := &protocol.Message{
		Action:  protocol.MessageAction("join"),
		Channel: channel,
	}
	return a.sendRequestAndProcessResponse(request)
}

func (a *RoomChatWS) Leave(channel string) (*protocol.Message, error) {
	request := &protocol.Message{
		Action:  protocol.MessageAction("leave"),
		Channel: channel,
	}
	return a.sendRequestAndProcessResponse(request)
}

func (a *RoomChatWS) SendMessage(message, channel string) (*protocol.Message, error) {
	request := &protocol.Message{
		Action:  protocol.MessageAction("message"),
		Channel: channel,
		Request: protocol.Request{
			Content: message,
		},
	}
	return a.sendRequestAndProcessResponse(request)
}
