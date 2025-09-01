package server

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zuczkows/room-chat/internal/chat"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second
	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second
	// Send pings to peer with this period. Must be less than pongWait
	pingPeriod = (pongWait * 9) / 10
	// Maximum message size allowed from peer
	maxMessageSize = 512
)

type ClientList map[*Client]bool

type Client struct {
	conn           *websocket.Conn
	manager        *Manager
	send           chan chat.Message
	user           string
	currentChannel string
	mu             sync.RWMutex
}

func NewClient(connection *websocket.Conn, manager *Manager) *Client {
	return &Client{
		conn:    connection,
		manager: manager,
		send:    make(chan chat.Message, 256),
	}
}

func (c *Client) SetCurrentChannel(channelName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.currentChannel = channelName
}

func (c *Client) GetCurrentChannel() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentChannel
}

func (c *Client) ClearCurrentChannel() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.currentChannel = ""
}

func (c *Client) Send() chan<- chat.Message {
	return c.send
}

func (c *Client) ReadMessages() {
	defer func() {
		c.manager.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, messageBytes, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		var message chat.Message
		if err := json.Unmarshal(messageBytes, &message); err != nil {
			log.Printf("error unmarshaling message: %v", err)
			continue
		}

		if c.user == "" && message.User != "" {
			c.user = message.User
		}

		c.manager.broadcast <- message
	}
}

// Note zuczkows - I used timeouts and queue from gorilla websockets example https://github.com/gorilla/websocket/blob/main/examples/chat/client.go
func (c *Client) WriteMessages() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}

			messageBytes, err := json.Marshal(message)
			if err != nil {
				log.Printf("error marshaling message: %v", err)
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
					log.Printf("error marshaling queued message: %v", err)
					continue
				}
				w.Write(nextMessageBytes)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
