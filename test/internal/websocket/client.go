package websocket

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Message struct {
	RequestID string `json:"request_id,omitempty"`
	Action    string `json:"action" validate:"required,oneof=join leave message login"`
	Type      string `json:"type,omitempty"`
	Channel   string `json:"channel,omitempty" validate:"min=1,max=50"`
	User      string `json:"user,omitempty"`
	Content   string `json:"content" validate:"max=500"`
	Token     string `json:"token,omitempty"`
	ClientID  string `json:"-"`
	Success   *bool  `json:"success,omitempty"`
	RespErr   *Err   `json:"error,omitempty"`
}

type Err struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type WSRequest struct {
	RequestID string `json:"request_id"`
	Action    string `json:"action"`
	Channel   string `json:"channel"`
	Token     string `json:"token"`
}

type WSError struct {
	Type    string
	Message string
	Raw     json.RawMessage
}

type WSClient struct {
	conn            *websocket.Conn
	mu              sync.RWMutex
	pendingRequests map[string]chan *Message
	pushStore       []*Message
	responseTimeout time.Duration
	user            string
	closed          bool
	done            chan struct{}
}

func NewWSClient(wsURL string, wsTimeout time.Duration, user string) (*WSClient, error) {
	log.Printf("Connecting to WebSocket URL: %s", wsURL)
	c, res, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusSwitchingProtocols {
		return nil, fmt.Errorf("websocket connection not opened successfully: %s", res.Status)
	}
	res.Body.Close()

	client := &WSClient{
		conn:            c,
		pendingRequests: make(map[string]chan *Message),
		pushStore:       make([]*Message, 0),
		responseTimeout: wsTimeout,
		user:            user,
		closed:          false,
		done:            make(chan struct{}),
	}
	go client.messageHandler()

	return client, nil
}

func (c *WSClient) messageHandler() {
	for {
		select {
		case <-c.done:
			return
		default:
			var msg Message
			err := c.conn.ReadJSON(&msg)
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure) ||
					strings.Contains(err.Error(), "use of closed network connection") {
					log.Printf("WebSocket connection closed normally %s", err.Error())
				} else {
					log.Printf("WebSocket error %s", err)
				}
				return
			}

			switch msg.Type {
			case "response":
				c.handleResponse(&msg)
			case "push":
				c.handlePush(&msg)
			default:
				log.Printf("Unknown message type: %s", msg.Type)
			}
		}
	}
}

func (c *WSClient) handleResponse(msg *Message) {
	if msg.RequestID == "" {
		return
	}

	c.mu.RLock()
	respChan, exists := c.pendingRequests[msg.RequestID]
	c.mu.RUnlock()

	if !exists {
		log.Printf("No pending request for request_id: %s", msg.RequestID)
		return
	}

	log.Printf("Received response %s, action %s, content %s",
		msg.RequestID, msg.Action, msg.Content)

	respChan <- msg
}

func (c *WSClient) handlePush(msg *Message) {
	log.Printf("Received push: action %s, user %s, content %s",
		msg.Action, msg.User, msg.Content)
	c.mu.Lock()
	c.pushStore = append(c.pushStore, msg)
	c.mu.Unlock()
}

// NOTE(zuczkows): to refactor - only works for login action now
func (c *WSClient) SendRequest(action string, token string) (*Message, error) {
	requestID := fmt.Sprintf("%d", rand.Int63())

	req := WSRequest{
		RequestID: requestID,
		Action:    action,
		Token:     token,
	}

	respChan := make(chan *Message, 1)

	c.mu.Lock()
	c.pendingRequests[requestID] = respChan
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pendingRequests, requestID)
		c.mu.Unlock()
		close(respChan)
	}()

	err := c.conn.WriteJSON(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	select {
	case response := <-respChan:
		return response, nil
	case <-time.After(c.responseTimeout):
		return nil, fmt.Errorf("timeout waiting for response to %s", action)
	}
}

func (c *WSClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	if c.conn != nil {
		close(c.done)
		c.conn.Close()
	}

	for requestID, respChan := range c.pendingRequests {
		close(respChan)
		delete(c.pendingRequests, requestID)
	}

	return nil
}

func (c *WSClient) GetPushes(push string) []*Message {
	c.mu.RLock()
	var pushes []*Message
	for _, msg := range c.pushStore {
		if msg.Action == push {
			pushes = append(pushes, msg)
		}
	}
	c.mu.RUnlock()
	return pushes
}

func (c *WSClient) ClearPushes() {
	c.mu.Lock()
	c.pushStore = make([]*Message, 0)
	c.mu.Unlock()
}

func (e *WSError) Error() string {
	return fmt.Sprintf("%s (type: %s)", e.Message, e.Type)
}

func HandleWSError(response *Message) error {
	return &WSError{
		Type:    response.RespErr.Type,
		Message: response.RespErr.Message,
	}
}
