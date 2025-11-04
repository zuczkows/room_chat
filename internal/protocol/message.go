package protocol

import (
	"github.com/go-playground/validator/v10"
)

type MessageAction string
type MessageType string

const (
	MesageActionJoin    MessageAction = "join"
	MessageActionLeave  MessageAction = "leave"
	MessageActionText   MessageAction = "message"
	MessageActionSystem MessageAction = "system"
	ErrorMessage        MessageAction = "error"
	LoginAction         MessageAction = "login"
)

const (
	MessageTypePush     MessageType = "push"
	MessageTypeResponse MessageType = "response"
)

var validate = validator.New()

type Message struct {
	RequestID string        `json:"request_id,omitempty"`
	Action    MessageAction `json:"action" validate:"required,oneof=join leave message login"`
	Type      MessageType   `json:"type,omitempty"`
	Channel   string        `json:"channel,omitempty"`
	User      string        `json:"user,omitempty"`
	Content   string        `json:"content,omitempty" validate:"max=500"`
	Token     string        `json:"token,omitempty"`
	ClientID  string        `json:"-"`
	Success   *bool         `json:"success,omitempty"`
	RespErr   *Err          `json:"error,omitempty"`
}

type Err struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// note from zuczkows - I think I should manually check struct and return nice
// error msg that i want instead of validate.Struct
func (m *Message) Validate() error {
	return validate.Struct(m)
}
