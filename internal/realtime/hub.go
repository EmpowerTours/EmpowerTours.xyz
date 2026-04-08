package realtime

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		// Allow mobile apps (no origin header) and known domains
		if origin == "" {
			return true // Mobile apps don't send Origin
		}
		allowed := []string{
			"https://empowertours.xyz",
			"https://www.empowertours.xyz",
			"https://api.empowertours.xyz",
			"http://localhost",
			"http://192.168.",
		}
		for _, a := range allowed {
			if len(origin) >= len(a) && origin[:len(a)] == a {
				return true
			}
		}
		return false
	},
}

// Message types sent over WebSocket
const (
	MsgTypeLocation    = "location"
	MsgTypeWaypoint    = "waypoint_reached"
	MsgTypeSessionStart = "session_start"
	MsgTypeSessionEnd  = "session_end"
	MsgTypePhoto       = "photo_taken"
	MsgTypePing        = "ping"
	MsgTypePong        = "pong"
)

// WSMessage is the envelope for all WebSocket messages.
type WSMessage struct {
	Type      string      `json:"type"`
	SessionID string      `json:"sessionId"`
	UserID    string      `json:"userId,omitempty"`
	Payload   interface{} `json:"payload"`
	Timestamp time.Time   `json:"timestamp"`
}

// LocationPayload is the GPS data sent by clients.
type LocationPayload struct {
	Latitude  float64  `json:"latitude"`
	Longitude float64  `json:"longitude"`
	Altitude  *float64 `json:"altitude,omitempty"`
	Accuracy  *float64 `json:"accuracy,omitempty"`
	Speed     *float64 `json:"speed,omitempty"`
	Heading   *float64 `json:"heading,omitempty"`
}

// Client represents a single WebSocket connection.
type Client struct {
	Hub       *Hub
	Conn      *websocket.Conn
	Send      chan []byte
	UserID    string
	SessionID string
}

// Hub manages all WebSocket connections and message broadcasting.
type Hub struct {
	// Sessions maps session IDs to connected clients
	sessions   map[string]map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan *WSMessage
	mu         sync.RWMutex
}

// NewHub creates a new WebSocket hub.
func NewHub() *Hub {
	return &Hub{
		sessions:   make(map[string]map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan *WSMessage, 256),
	}
}

// Run starts the hub's main event loop.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if h.sessions[client.SessionID] == nil {
				h.sessions[client.SessionID] = make(map[*Client]bool)
			}
			h.sessions[client.SessionID][client] = true
			h.mu.Unlock()
			log.Printf("Client %s joined session %s (%d clients)",
				client.UserID, client.SessionID, len(h.sessions[client.SessionID]))

		case client := <-h.unregister:
			h.mu.Lock()
			if clients, ok := h.sessions[client.SessionID]; ok {
				if _, exists := clients[client]; exists {
					delete(clients, client)
					close(client.Send)
					if len(clients) == 0 {
						delete(h.sessions, client.SessionID)
					}
				}
			}
			h.mu.Unlock()
			log.Printf("Client %s left session %s", client.UserID, client.SessionID)

		case msg := <-h.broadcast:
			h.mu.RLock()
			clients := h.sessions[msg.SessionID]
			h.mu.RUnlock()

			data, err := json.Marshal(msg)
			if err != nil {
				log.Printf("Failed to marshal broadcast: %v", err)
				continue
			}

			for client := range clients {
				select {
				case client.Send <- data:
				default:
					h.mu.Lock()
					delete(h.sessions[msg.SessionID], client)
					close(client.Send)
					h.mu.Unlock()
				}
			}
		}
	}
}

// Broadcast sends a message to all clients in a session.
func (h *Hub) Broadcast(msg *WSMessage) {
	h.broadcast <- msg
}

// SessionClientCount returns the number of connected clients for a session.
func (h *Hub) SessionClientCount(sessionID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.sessions[sessionID])
}

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 4096
)

// readPump reads messages from the WebSocket connection.
func (c *Client) readPump() {
	defer func() {
		c.Hub.unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("Invalid message from %s: %v", c.UserID, err)
			continue
		}

		// Handle ping
		if msg.Type == MsgTypePing {
			pong := WSMessage{Type: MsgTypePong, SessionID: c.SessionID, Timestamp: time.Now()}
			data, _ := json.Marshal(pong)
			c.Send <- data
			continue
		}

		// Set metadata and rebroadcast
		msg.SessionID = c.SessionID
		msg.UserID = c.UserID
		msg.Timestamp = time.Now()
		c.Hub.Broadcast(&msg)
	}
}

// writePump writes messages to the WebSocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ServeWS upgrades an HTTP connection to WebSocket and registers the client.
func ServeWS(hub *Hub, userID, sessionID string, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	client := &Client{
		Hub:       hub,
		Conn:      conn,
		Send:      make(chan []byte, 256),
		UserID:    userID,
		SessionID: sessionID,
	}

	hub.register <- client

	go client.writePump()
	go client.readPump()
}
