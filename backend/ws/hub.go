package ws

import (
	"log"
	"net/http"
	"sync"

	"backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for dev
	},
}

// Client represents a connected user session over WebSocket
type Client struct {
	UserID string
	Conn   *websocket.Conn
	Send   chan []byte
}

// Hub maintains the set of active clients and handles routing messages
type Hub struct {
	clients    map[string]map[*Client]bool // userID -> active WS connections map
	register   chan *Client
	unregister chan *Client
	broadcast  chan MessageEnvelope
	mu         sync.RWMutex
}

type MessageEnvelope struct {
	UserID  string `json:"user_id"`
	Payload []byte `json:"payload"`
}

var GlobalHub *Hub

func InitHub() {
	GlobalHub = &Hub{
		clients:    make(map[string]map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan MessageEnvelope),
	}
	go GlobalHub.Run()
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if _, ok := h.clients[client.UserID]; !ok {
				h.clients[client.UserID] = make(map[*Client]bool)
			}
			h.clients[client.UserID][client] = true
			h.mu.Unlock()
			log.Printf("WebSocket Client connected: user %s", client.UserID)

		case client := <-h.unregister:
			h.mu.Lock()
			if connections, ok := h.clients[client.UserID]; ok {
				if _, exists := connections[client]; exists {
					delete(connections, client)
					close(client.Send)
					client.Conn.Close()
					log.Printf("WebSocket Client disconnected: user %s", client.UserID)
				}
				if len(connections) == 0 {
					delete(h.clients, client.UserID)
				}
			}
			h.mu.Unlock()

		case env := <-h.broadcast:
			h.mu.RLock()
			connections, ok := h.clients[env.UserID]
			h.mu.RUnlock()
			if ok {
				for client := range connections {
					select {
					case client.Send <- env.Payload:
					default:
						h.mu.Lock()
						delete(connections, client)
						close(client.Send)
						client.Conn.Close()
						h.mu.Unlock()
					}
				}
			}
		}
	}
}

// BroadcastToUser sends a real-time message payload to all active WS sessions for a user
func BroadcastToUser(userID string, payload []byte) {
	if GlobalHub == nil {
		return
	}
	GlobalHub.broadcast <- MessageEnvelope{
		UserID:  userID,
		Payload: payload,
	}
}

// HandleWS upgrades HTTP to WebSocket and registers the client
func HandleWS(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Query parameter token is required"})
		return
	}

	// Verify token
	claims, err := utils.ParseJWT(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
		return
	}

	userID, ok := claims["sub"].(string)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
		return
	}

	// Upgrade connection
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	client := &Client{
		UserID: userID,
		Conn:   conn,
		Send:   make(chan []byte, 256),
	}

	GlobalHub.register <- client

	// Start write and read pumps in separate goroutines
	go client.writePump()
	go client.readPump()
}

// readPump pumps messages from the websocket connection to the hub (we just keep connection alive, discard input)
func (c *Client) readPump() {
	defer func() {
		GlobalHub.unregister <- c
	}()
	for {
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// writePump pumps messages from the hub to the websocket connection
func (c *Client) writePump() {
	defer func() {
		c.Conn.Close()
	}()
	for {
		message, ok := <-c.Send
		if !ok {
			// Hub closed the channel
			_ = c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		}

		err := c.Conn.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			break
		}
	}
}
