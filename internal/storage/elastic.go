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

const (
	maxRetries             = 3
	retryTimeoutMilisecond = time.Millisecond * 1
)

type IndexedMessage struct {
	ID        string    `json:"message_id"`
	ChannelID string    `json:"channel_id"`
	AuthorID  string    `json:"author_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type EsResponse struct {
	Hits struct {
		Hits []Hit `json:"hits"`
	} `json:"hits"`
}

type Hit struct {
	Source IndexedMessage `json:"_source"`
}

type CreateIndexRequest struct {
	Settings ESSettings `json:"settings"`
	Mappings ESMappings `json:"mappings"`
}

type ESSettings struct {
	NumberOfShards   int `json:"number_of_shards"`
	NumberOfReplicas int `json:"number_of_replicas"`
}

type ESMappings struct {
	Properties ESMappingProperties `json:"properties"`
}

type ESMappingProperties struct {
	MessageID ESField `json:"message_id"`
	ChannelID ESField `json:"channel_id"`
	AuthorID  ESField `json:"author_id"`
	Content   ESField `json:"content"`
	CreatedAt ESField `json:"created_at"`
}

type ESField struct {
	Type string `json:"type"`
}

type CreateQuery struct {
	Query ESQuery     `json:"query"`
	Sort  []SortQuery `json:"sort"`
}

type ESQuery struct {
	Term Term `json:"term"`
}

type Term struct {
	ChannelID string `json:"channel_id"`
}

type SortQuery struct {
	CreatedAt SortOrder `json:"created_at"`
}

type SortOrder struct {
	Order string `json:"order"`
}

type MessageIndexer struct {
	db     *elasticsearch.Client
	logger *slog.Logger
	index  string
}

func NewMessageIndexer(db *elasticsearch.Client, logger *slog.Logger, index string) *MessageIndexer {
	return &MessageIndexer{
		db:     db,
		logger: logger,
		index:  index,
	}
}

func (es *MessageIndexer) IndexMessage(message protocol.Message) error {
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

	for attempt := 1; attempt <= maxRetries; attempt++ {
		res, err := es.db.Index(
			es.index,
			bytes.NewReader(body),
			es.db.Index.WithDocumentID(msg.ID),
		)
		if err != nil {
			es.logger.Warn("indexing message failed", slog.String("messageID", msg.ID), slog.Int("attempt", attempt))
			if attempt == maxRetries {
				es.logger.Error("indexing failed after all retries", slog.String("messageID", msg.ID), slog.Any("error", err))
				return fmt.Errorf("es indexing error: %w", err)
			}
			time.Sleep(retryTimeoutMilisecond)
			continue
		}
		defer res.Body.Close()
		return nil

	}
	return nil
}

func (es *MessageIndexer) ListDocuments(channel string) ([]IndexedMessage, error) {
	es.logger.Debug("Calling List documents", slog.String("channel", channel))

	query := CreateQuery{
		Query: ESQuery{
			Term: Term{
				ChannelID: channel,
			},
		},
		Sort: []SortQuery{
			{
				CreatedAt: SortOrder{Order: "desc"},
			},
		},
	}

	byteQuery, err := json.Marshal(query)
	if err != nil {
		es.logger.Error("error marshaling query", slog.Any("error", err))
		return nil, fmt.Errorf("marshal error: %w", err)
	}

	res, err := es.db.Search(
		es.db.Search.WithBody(bytes.NewReader(byteQuery)),
		es.db.Search.WithIndex(es.index),
	)
	if err != nil {
		return nil, fmt.Errorf("es search error: %w", err)
	}
	defer res.Body.Close()

	var esResponse EsResponse

	if err = json.NewDecoder(res.Body).Decode(&esResponse); err != nil {
		return nil, fmt.Errorf("failed to decode es response: %w", err)
	}

	messages := make([]IndexedMessage, 0, len(esResponse.Hits.Hits))
	for _, h := range esResponse.Hits.Hits {
		messages = append(messages, h.Source)
	}
	return messages, nil
}

// NOTE(zuczkows): Required for integration tests - without that first IndexMessage creates mapping automatically and ListDocuments does not work properly
func (es *MessageIndexer) CreateIndex() error {
	es.logger.Debug("Creating ES Index", slog.String("index", es.index))
	body := CreateIndexRequest{
		Settings: ESSettings{
			NumberOfShards:   2,
			NumberOfReplicas: 0,
		},
		Mappings: ESMappings{
			Properties: ESMappingProperties{
				MessageID: ESField{Type: "keyword"},
				ChannelID: ESField{Type: "keyword"},
				AuthorID:  ESField{Type: "keyword"},
				Content:   ESField{Type: "text"},
				CreatedAt: ESField{Type: "date"},
			},
		},
	}

	byteBody, err := json.Marshal(body)
	if err != nil {
		es.logger.Error("error marshaling CreateIndex body", slog.Any("error", err))
		return fmt.Errorf("marshal error: %w", err)
	}

	res, err := es.db.Indices.Create(
		es.index,
		es.db.Indices.Create.WithBody(bytes.NewReader(byteBody)),
	)
	if err != nil {
		return fmt.Errorf("error creating index: %w", err)
	}
	defer res.Body.Close()
	return nil
}
