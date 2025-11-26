package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
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

type EsResponse struct {
	Hits HitsExternal `json:"hits"`
}

type HitsExternal struct {
	Hits []Hit `json:"hits"`
}

type Hit struct {
	Source IndexedMessage `json:"_source"`
}

type MessageIndexer struct {
	db     *elasticsearch.Client
	logger *slog.Logger
	index  string
}

func NewMessageIndexer(db *elasticsearch.Client, logger *slog.Logger) *MessageIndexer {
	return &MessageIndexer{
		db:     db,
		logger: logger,
		index:  "test3",
	}
}

func (es *MessageIndexer) IndexWSMessage(message protocol.Message) error {
	es.logger.Debug("Calling Index WS Message", slog.String("channel", message.Channel))
	msg := IndexedMessage{
		ID:        message.ID,
		ChannelID: message.Channel,
		AuthorID:  message.User,
		Content:   message.Request.Content,
		CreatedAt: message.CreatedAt,
	}

	body, err := json.Marshal(msg)
	if err != nil {
		es.logger.Error("error marshaling message", slog.Any("error", err))
		return fmt.Errorf("marshal error: %w", err)
	}

	res, err := es.db.Index(
		es.index,
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

func (es *MessageIndexer) ListDocuments(channel string) ([]IndexedMessage, error) {
	es.logger.Debug("Calling List documents", slog.String("channel", channel))

	query := fmt.Sprintf(`
	{
		"query": {
			"term": {
				"channel_id": "%s"
			}
		},
		"sort": [
			{ "created_at": { "order": "desc" } }
		]
	}`, channel)

	res, err := es.db.Search(
		es.db.Search.WithBody(bytes.NewBufferString(query)),
		es.db.Search.WithIndex(es.index),
	)
	if err != nil {
		return nil, fmt.Errorf("es search error: %w", err)
	}
	defer res.Body.Close()

	bodyBytes, _ := io.ReadAll(res.Body)
	var esResponse EsResponse

	if err := json.Unmarshal(bodyBytes, &esResponse); err != nil {
		return nil, fmt.Errorf("failed to decode es response: %w", err)
	}

	messages := make([]IndexedMessage, 0, len(esResponse.Hits.Hits))
	for _, h := range esResponse.Hits.Hits {
		messages = append(messages, h.Source)
	}
	return messages, nil
}

// NOTE(zuczkows): Required for integration tests - without that first IndexWSMessage creates mapping automatically and ListDocuments does not work properly
func (es *MessageIndexer) CreateIndex() error {
	es.logger.Debug("Creating ES Index", slog.String("index", es.index))
	query := `
	{
		"settings": {
			"number_of_shards": 2,
			"number_of_replicas": 0
		},
		"mappings": {
			"properties": {
				"message_id": { "type": "keyword" },
				"channel_id": { "type": "keyword" },
				"author_id":  { "type": "keyword" },
				"content":    { "type": "text" },
				"created_at":  { "type": "date" }
			}
		}
	}`
	res, err := es.db.Indices.Create(
		es.index,
		es.db.Indices.Create.WithBody(strings.NewReader(query)),
	)
	if err != nil {
		return fmt.Errorf("Error creating index: %w", err)
	}
	defer res.Body.Close()
	return nil
}
