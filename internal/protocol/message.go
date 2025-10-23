package protocol

import (
	"github.com/go-playground/validator/v10"
)

type MessageAction string

const (
	MesageActionJoin    MessageAction = "join"
	MessageActionLeave  MessageAction = "leave"
	MessageActionText   MessageAction = "message"
	MessageActionSystem MessageAction = "system"
	ErrorMessage        MessageAction = "error"
	LoginAction         MessageAction = "login"
)

var validate = validator.New()

type Message struct {
	Type     MessageAction `json:"type" validate:"required,oneof=join leave message login"`
	ClientID string        `json:"-"`
	Channel  string        `json:"channel,omitempty" validate:"min=1,max=50"`
	User     string        `json:"user,omitempty"`
	Content  string        `json:"content" validate:"max=500"`
	Token    string        `json:"token,omitempty"`
}

// note from zuczkows - I think I should manually check struct and return nice
// error msg that i want instead of validate.Struct
func (m *Message) Validate() error {
	return validate.Struct(m)
}
