package websocket

import (
	"fmt"
	"strings"

	"github.com/zuczkows/room-chat/internal/protocol"
)

type PushCriteria struct {
	Action  protocol.MessageAction `json:"action,omitempty"`
	Content string                 `json:"content,omitempty"`
}

func PushDelivered(client *WSClient, criteria PushCriteria) func() bool {
	return func() bool {
		pushes := client.GetPushes(criteria.Action)

		for _, push := range pushes {
			if matchesPush(*push, criteria) {
				fmt.Printf("Found matching push: action=%s, content=%s", push.Action, push.Push.Content)
				return true
			}
		}

		return false
	}
}

func matchesPush(push protocol.Message, criteria PushCriteria) bool {
	if criteria.Content != "" && push.Push != nil {
		if !strings.Contains(push.Push.Content, criteria.Content) {
			return false
		}
	}

	return true
}
