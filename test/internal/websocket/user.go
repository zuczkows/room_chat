package websocket

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
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

type loginResPayload struct {
	RequestID string `json:"request_id"`
	Action    string `json:"action"`
	Type      string `json:"type"`
	Content   string `json:"content"`
}

func (a *RoomChatWS) Login(accessToken string) (*loginResPayload, error) {
	response, err := a.SendRequest("login", accessToken)
	if err != nil {
		return nil, err
	}
	if response.Success == nil || !*response.Success {
		return nil, HandleWSError(response)
	}
	resBytes, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}

	resPayload := &loginResPayload{}
	if err := json.Unmarshal(resBytes, resPayload); err != nil {
		return nil, err
	}

	return resPayload, nil
}
