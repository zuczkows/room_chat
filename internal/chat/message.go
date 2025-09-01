package chat

type MessageAction string

const (
	MesageActionJoin     MessageAction = "join"
	MessageActionLeave   MessageAction = "leave"
	MessageActionMessage MessageAction = "message"
)

type Message struct {
	Type    MessageAction `json:"type"`
	Channel string        `json:"channel"`
	User    string        `json:"user"`
	Content string        `json:"content"`
}
