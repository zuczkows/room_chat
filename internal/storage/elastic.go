package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/zuczkows/room-chat/internal/protocol"
)

type IndexedMessage struct {
	ID        string    `json:"message_id"`
	ChannelID string    `json:"channel_id"`
	AuthorID  string    `json:"author_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type MessageIndexer struct {
	db     *elasticsearch.Client
	logger *slog.Logger
}

func NewMessageIndexer(db *elasticsearch.Client, logger *slog.Logger) *MessageIndexer {
	return &MessageIndexer{
		db:     db,
		logger: logger,
	}
}

func (es *MessageIndexer) IndexWSMessage(message protocol.Message) error {
	msg := IndexedMessage{
		ID:        message.ID,
		ChannelID: message.Channel,
		AuthorID:  message.User,
		Content:   message.Message,
		CreatedAt: message.CreatedAt,
	}

	body, err := json.Marshal(msg)
	if err != nil {
		es.logger.Error("error marshaling message", slog.Any("error", err))
		return fmt.Errorf("marshal error: %w", err)
	}

	res, err := es.db.Index(
		"test2",
		bytes.NewReader(body),
		es.db.Index.WithDocumentID(msg.ID),
	)
	if err != nil {
		es.logger.Error("indexing error", slog.Any("error", err))
		return fmt.Errorf("es indexing error: %w", err)
	}
	defer res.Body.Close()

	return nil
}
