package chat

import "github.com/go-playground/validator/v10"

type MessageAction string

const (
	MesageActionJoin     MessageAction = "join"
	MessageActionLeave   MessageAction = "leave"
	MessageActionMessage MessageAction = "message"
	MessageActionSystem  MessageAction = "system"
)

type Message struct {
	Type    MessageAction `json:"type" validate:"required,oneof=join leave message"`
	Channel string        `json:"channel" validate:"required,min=1,max=50"`
	User    string        `json:"user" validate:"required,min=1,max=30"`
	Content string        `json:"content" validate:"max=500"`
}

// note from zuczkows - I think I should manually check struct and return nice
// error msg that i want instead of validate.Struct
func (m *Message) Validate() error {
	validate := validator.New()
	return validate.Struct(m)
}
