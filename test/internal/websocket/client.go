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
	"github.com/zuczkows/room-chat/internal/protocol"
)

type WSError struct {
	Type    protocol.ErrorType
	Message string
	Raw     json.RawMessage
}

type WSClient struct {
	conn            *websocket.Conn
	mu              sync.RWMutex
	pendingRequests map[string]chan *protocol.Message
	pushStore       []*protocol.Message
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
		pendingRequests: make(map[string]chan *protocol.Message),
		pushStore:       make([]*protocol.Message, 0),
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
			var msg protocol.Message
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

func (c *WSClient) handleResponse(msg *protocol.Message) {
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
		msg.RequestID, msg.Action, msg.Response.Content)

	respChan <- msg
}

func (c *WSClient) handlePush(msg *protocol.Message) {
	log.Printf("Received push: action %s, user %s, content %s",
		msg.Action, msg.User, msg.Push.Content)
	c.mu.Lock()
	c.pushStore = append(c.pushStore, msg)
	c.mu.Unlock()
}

func (c *WSClient) SendRequest(req *protocol.Message) (*protocol.Message, error) {
	requestID := fmt.Sprintf("%d", rand.Int63())
	req.RequestID = requestID
	req.Type = protocol.MessageTypeRequest

	respChan := make(chan *protocol.Message, 1)

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
		return nil, fmt.Errorf("timeout waiting for response to %s", req.Action)
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

func (c *WSClient) GetPushes(action protocol.MessageAction) []*protocol.Message {
	c.mu.RLock()
	var pushes []*protocol.Message
	for _, msg := range c.pushStore {
		if msg.Action == protocol.MessageAction(action) {
			pushes = append(pushes, msg)
		}
	}
	c.mu.RUnlock()
	return pushes
}

func (c *WSClient) ClearPushes() {
	c.mu.Lock()
	c.pushStore = make([]*protocol.Message, 0)
	c.mu.Unlock()
}

func (e *WSError) Error() string {
	return fmt.Sprintf("%s (type: %s)", e.Message, e.Type)
}

func HandleWSError(response *protocol.Message) error {
	return &WSError{
		Type:    response.Response.RespErr.Type,
		Message: response.Response.RespErr.Message,
	}
}
