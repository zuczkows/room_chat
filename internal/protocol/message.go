package protocol

import (
	"time"

	"github.com/go-playground/validator/v10"
)

type MessageAction string
type MessageType string
type ErrorType string

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

const (
	AuthorizationError  ErrorType = "authorization"
	ValidationError     ErrorType = "validation"
	ForbiddenError      ErrorType = "forbidden"
	InternalServerError ErrorType = "internal server error"
	ConflictError       ErrorType = "conflict"
)

var validate = validator.New()

type Message struct {
	ID        string        `json:"message_id,omitempty"`
	Action    MessageAction `json:"action" validate:"required,oneof=join leave message login"`
	Type      MessageType   `json:"type,omitempty"`
	ClientID  string        `json:"-"`
	User      string        `json:"user,omitempty"`
	RequestID string        `json:"request_id,omitempty"`
	Channel   string        `json:"channel,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	Response  *Response     `json:"response,omitempty"`
	Push      *Push         `json:"push,omitempty"`
	Request
}

type Response struct {
	Content string        `json:"content,omitempty" validate:"max=500"`
	Success bool          `json:"success"`
	RespErr *ErrorDetails `json:"error,omitempty"`
}

type Push struct {
	Content string `json:"content,omitempty" validate:"max=500"`
}

type Request struct {
	Content string `json:"content,omitempty" validate:"max=500"`
	Token   string `json:"token,omitempty"`
}

type ErrorDetails struct {
	Type    ErrorType `json:"type"`
	Message string    `json:"message"`
}

// note from zuczkows - I think I should manually check struct and return nice
// error msg that i want instead of validate.Struct
func (m *Message) Validate() error {
	return validate.Struct(m)
}
