package connection

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/zuczkows/room-chat/internal/protocol"
)

const (
	WriteWait      = 10 * time.Second
	PongWait       = 60 * time.Second
	PingInterval   = (PongWait * 9) / 10 // Must be less than PongWait
	MaxMessageSize = 512
)

type Client struct {
	conn          *websocket.Conn
	ConnID        string
	closeCh       chan<- *Client
	dispatchCh    chan<- protocol.Message
	send          chan protocol.Message
	user          string
	mu            sync.RWMutex
	logger        *slog.Logger
	authenticated bool
	authOnce      sync.Once
}

func NewClient(connection *websocket.Conn, closeCh chan<- *Client, dispatchCh chan<- protocol.Message, logger *slog.Logger) *Client {
	return &Client{
		conn:          connection,
		ConnID:        uuid.New().String(),
		closeCh:       closeCh,
		dispatchCh:    dispatchCh,
		send:          make(chan protocol.Message, 256),
		logger:        logger,
		authenticated: false,
	}
}

func (c *Client) Authenticate(username string) {
	c.authOnce.Do(func() {
		c.user = username
		c.authenticated = true
	})
}

func (c *Client) IsAuthenticated() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.authenticated
}

func (c *Client) GetUser() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.user
}

func (c *Client) Send() chan<- protocol.Message {
	return c.send
}

func (c *Client) ReadMessages() {
	defer func() {
		c.closeCh <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(MaxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(PongWait))
	c.conn.SetPongHandler(c.pongHandler)

	for {
		_, messageBytes, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.logger.Error("Unexpected WebSocket close error", slog.Any("error", err))
			}
			break
		}
		var message protocol.Message
		if err := json.Unmarshal(messageBytes, &message); err != nil {
			c.logger.Error("error marshaling message", slog.Any("error", err))
			errorMsg := protocol.Message{
				Action: protocol.ErrorMessage,
				Response: &protocol.Response{
					Content: fmt.Sprintf("Message validation failed: %v", err),
				},
			}
			c.send <- errorMsg
			continue
		}

		if message.Action != protocol.LoginAction && !c.IsAuthenticated() {
			c.logger.Error("user not authenticated")
			errorMsg := protocol.Message{
				Action: protocol.ErrorMessage,
				Response: &protocol.Response{
					Content: "You are not logged in",
				},
			}
			c.send <- errorMsg
			continue
		}

		if err := message.Validate(); err != nil {
			c.logger.Error("message validation failed",
				slog.Any("error", err),
				slog.String("user", c.GetUser()))

			errorMsg := protocol.Message{
				Action: protocol.ErrorMessage,
				Response: &protocol.Response{
					Content: fmt.Sprintf("Message validation failed: %v", err),
				},
			}
			c.send <- errorMsg
			continue
		}
		message.User = c.GetUser()
		message.ClientID = c.ConnID

		c.dispatchCh <- message
	}
}

// Note zuczkows - I used timeouts and queue from gorilla websockets example https://github.com/gorilla/websocket/blob/main/examples/chat/client.go
func (c *Client) WriteMessages() {
	ticker := time.NewTicker(PingInterval)
	defer func() {
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(WriteWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}

			messageBytes, err := json.Marshal(message)
			if err != nil {
				c.logger.Error("error marshaling queued message", slog.Any("error", err))
				return
			}
			w.Write(messageBytes)

			// Add queued chat messages to the current websocket message
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				nextMessage := <-c.send
				nextMessageBytes, err := json.Marshal(nextMessage)
				if err != nil {
					c.logger.Error("error marshaling queued message", slog.Any("error", err))
					continue
				}
				w.Write(nextMessageBytes)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(WriteWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) pongHandler(pongMsg string) error {
	return c.conn.SetReadDeadline(time.Now().Add(PongWait))
}
