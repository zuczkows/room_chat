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
	MessageTypeRequest  MessageType = "request"
	MessageTypeResponse MessageType = "response"
)

var validate = validator.New()

type Message struct {
	Action    MessageAction `json:"action" validate:"required,oneof=join leave message login"`
	Type      MessageType   `json:"type,omitempty"`
	ClientID  string        `json:"-"`
	User      string        `json:"user,omitempty"`
	RequestID string        `json:"request_id,omitempty"`
	Channel   string        `json:"channel,omitempty"`
	*Response `json:"response,omitempty"`
	*Push     `json:"push,omitempty"`
	Request
}

type Response struct {
	Content string `json:"content,omitempty" validate:"max=500"`
	Success bool   `json:"success"`
	RespErr *Err   `json:"error,omitempty"`
}

type Push struct {
	Content string `json:"content,omitempty" validate:"max=500"`
}

type Request struct {
	Message string `json:"message,omitempty" validate:"max=500"`
	Token   string `json:"token,omitempty"`
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
