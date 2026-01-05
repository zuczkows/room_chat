package elastic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esapi"
	"github.com/sethvargo/go-retry"
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

type SearchResponse struct {
	Hits struct {
		Hits []Hit `json:"hits"`
	} `json:"hits"`
}

type Hit struct {
	Source IndexedMessage `json:"_source"`
}

type SearchQuery struct {
	Query Query               `json:"query"`
	Aggs  map[string]AggQuery `json:"aggs,omitempty"`
	Sort  []SortQuery         `json:"sort,omitempty"`
	Size  int                 `json:"size,omitempty"`
}

type AggQuery struct {
	DateHistogram DateHistogram `json:"date_histogram"`
}

type DateHistogram struct {
	Field         string `json:"field"`
	FixedInterval string `json:"fixed_interval"`
	MinDocCount   int64  `json:"min_doc_count"`
}

type AggSearchResponse struct {
	Aggregations map[string]struct {
		Buckets []Bucket `json:"buckets"`
	} `json:"aggregations"`
}

type Bucket struct {
	KeyAsString string `json:"key_as_string"`
	Key         int64  `json:"key"`
	DocCount    int64  `json:"doc_count"`
}

type BucketResults struct {
	Key   time.Time
	Count int64
}

type Query struct {
	Bool BoolQuery `json:"bool"`
}

type BoolQuery struct {
	Filter []TermQuery `json:"filter"`
}

type TermQuery struct {
	Term map[string]string `json:"term"`
}

type SortQuery struct {
	CreatedAt SortOrder `json:"created_at"`
}

type SortOrder struct {
	Order string `json:"order"`
}

type ListOptions struct {
	AuthorID string `json:"author_id"`
}

type ListOption func(*ListOptions)

func WithAuthorID(authorID string) ListOption {
	return func(o *ListOptions) {
		o.AuthorID = authorID
	}
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

	err = retry.Do(context.Background(), retry.WithMaxRetries(maxRetries, retry.NewConstant(retryTimeoutMilisecond)), func(ctx context.Context) error {
		res, err := es.db.Index(
			es.index,
			bytes.NewReader(body),
			es.db.Index.WithDocumentID(msg.ID),
		)

		if err != nil {
			es.logger.Warn("indexing message failed", slog.String("messageID", msg.ID), slog.Any("error", err))
			return retry.RetryableError(err)
		}

		defer res.Body.Close()

		if res.IsError() {
			return parseESError(res)
		}
		return nil
	})

	if err != nil {
		es.logger.Error("indexing failed after retries", slog.String("messageID", msg.ID), slog.Any("error", err))
		return fmt.Errorf("es indexing error: %w", err)
	}

	return nil
}

func (es *MessageIndexer) ListDocuments(channel string, opts ...ListOption) ([]IndexedMessage, error) {
	es.logger.Debug("Calling List documents", slog.String("channel", channel))

	var o ListOptions
	for _, opt := range opts {
		opt(&o)
	}

	filters := []TermQuery{
		{Term: map[string]string{"channel_id": channel}},
	}
	if o.AuthorID != "" {
		filters = append(filters, TermQuery{Term: map[string]string{"author_id": o.AuthorID}})
	}

	query := SearchQuery{
		Query: Query{
			Bool: BoolQuery{
				Filter: filters,
			},
		},
		Sort: []SortQuery{
			{CreatedAt: SortOrder{Order: "desc"}},
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
	if res.IsError() {
		return nil, parseESError(res)
	}

	var searchResponse SearchResponse

	if err = json.NewDecoder(res.Body).Decode(&searchResponse); err != nil {
		return nil, fmt.Errorf("failed to decode es response: %w", err)
	}

	messages := make([]IndexedMessage, 0, len(searchResponse.Hits.Hits))
	for _, h := range searchResponse.Hits.Hits {
		messages = append(messages, h.Source)
	}
	return messages, nil
}

func (es *MessageIndexer) GetMessageStats(channel, authorID, fixedInterval string) ([]BucketResults, error) {
	q := SearchQuery{
		Query: Query{
			Bool: BoolQuery{
				Filter: []TermQuery{
					{Term: map[string]string{"channel_id": channel}},
					{Term: map[string]string{"author_id": authorID}},
				},
			},
		},
		Aggs: map[string]AggQuery{
			"messages_over_time": {
				DateHistogram: DateHistogram{
					Field:         "created_at",
					FixedInterval: fixedInterval,
					MinDocCount:   1,
				},
			},
		},
		Size: 0,
	}
	body, err := json.Marshal(q)
	if err != nil {
		return nil, fmt.Errorf("marshal stats query: %w", err)
	}

	res, err := es.db.Search(
		es.db.Search.WithIndex(es.index),
		es.db.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return nil, fmt.Errorf("es search error: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, parseESError(res)
	}

	var AggResponse AggSearchResponse
	if err := json.NewDecoder(res.Body).Decode(&AggResponse); err != nil {
		return nil, fmt.Errorf("decode stats response: %w", err)
	}

	agg, ok := AggResponse.Aggregations["messages_over_time"]
	if !ok {
		return nil, fmt.Errorf("missing aggregation: messages_over_time")
	}

	buckets := make([]BucketResults, 0, len(agg.Buckets))
	for _, b := range agg.Buckets {
		buckets = append(buckets, BucketResults{
			Key:   time.UnixMilli(b.Key).UTC(),
			Count: b.DocCount,
		})
	}
	return buckets, nil
}

func parseESError(res *esapi.Response) error {
	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return retry.RetryableError(err)
	}
	return retry.RetryableError(fmt.Errorf("es error: status=%d body=%s", res.StatusCode, string(bodyBytes)))
}
