package ws

import (
	"log"
	"sync"

	"github.com/gofiber/contrib/websocket"
)

// TargetedMessage represents a message to be sent to specific users
type TargetedMessage struct {
	UserIDs []string
	Message []byte
}

type Hub struct {
	// All clients (for broadcast to all)
	Clients map[*websocket.Conn]bool

	// Map user ID to their connections for targeted messaging
	// A user might have multiple connections (multiple tabs/devices)
	UserClients map[string][]*websocket.Conn

	// Map connection to user ID for cleanup
	ConnToUser map[*websocket.Conn]string

	Register   chan *websocket.Conn
	Unregister chan *websocket.Conn
	Broadcast  chan []byte

	// Targeted message channel
	TargetedSend chan *TargetedMessage

	mutex sync.Mutex
}

func NewHub() *Hub {
	return &Hub{
		Clients:      make(map[*websocket.Conn]bool),
		UserClients:  make(map[string][]*websocket.Conn),
		ConnToUser:   make(map[*websocket.Conn]string),
		Register:     make(chan *websocket.Conn),
		Unregister:   make(chan *websocket.Conn),
		Broadcast:    make(chan []byte),
		TargetedSend: make(chan *TargetedMessage),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case conn := <-h.Register:
			h.mutex.Lock()
			h.Clients[conn] = true
			h.mutex.Unlock()
			log.Println("New WS Client Connected (anonymous)")

		case conn := <-h.Unregister:
			h.mutex.Lock()
			if _, ok := h.Clients[conn]; ok {
				delete(h.Clients, conn)

				// Also remove from UserClients if registered
				if userID, exists := h.ConnToUser[conn]; exists {
					h.removeUserConnection(userID, conn)
					delete(h.ConnToUser, conn)
				}

				conn.Close()
			}
			h.mutex.Unlock()

		case message := <-h.Broadcast:
			h.mutex.Lock()
			for conn := range h.Clients {
				if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
					conn.Close()
					delete(h.Clients, conn)
					// Cleanup user mapping
					if userID, exists := h.ConnToUser[conn]; exists {
						h.removeUserConnection(userID, conn)
						delete(h.ConnToUser, conn)
					}
				}
			}
			h.mutex.Unlock()

		case targeted := <-h.TargetedSend:
			h.mutex.Lock()
			for _, userID := range targeted.UserIDs {
				if conns, exists := h.UserClients[userID]; exists {
					for _, conn := range conns {
						if err := conn.WriteMessage(websocket.TextMessage, targeted.Message); err != nil {
							conn.Close()
							delete(h.Clients, conn)
							h.removeUserConnection(userID, conn)
							delete(h.ConnToUser, conn)
						}
					}
				}
			}
			h.mutex.Unlock()
		}
	}
}

// RegisterWithUser registers a connection with a specific user ID for targeted messaging
func (h *Hub) RegisterWithUser(conn *websocket.Conn, userID string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	// Add to general clients
	h.Clients[conn] = true

	// Add to user-specific connections
	h.UserClients[userID] = append(h.UserClients[userID], conn)
	h.ConnToUser[conn] = userID

	log.Printf("WS Client Connected for user: %s (total connections: %d)", userID, len(h.UserClients[userID]))
}

// SendToUsers sends a message only to specific users
func (h *Hub) SendToUsers(userIDs []string, message []byte) {
	h.TargetedSend <- &TargetedMessage{
		UserIDs: userIDs,
		Message: message,
	}
}

// removeUserConnection removes a specific connection from a user's connection list
func (h *Hub) removeUserConnection(userID string, conn *websocket.Conn) {
	if conns, exists := h.UserClients[userID]; exists {
		for i, c := range conns {
			if c == conn {
				// Remove this connection from the slice
				h.UserClients[userID] = append(conns[:i], conns[i+1:]...)
				break
			}
		}
		// If no more connections for this user, remove the user entry
		if len(h.UserClients[userID]) == 0 {
			delete(h.UserClients, userID)
		}
	}
}

// GetUserConnectionCount returns the number of active connections for a user
func (h *Hub) GetUserConnectionCount(userID string) int {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	return len(h.UserClients[userID])
}

// IsUserOnline checks if a user has any active WebSocket connections
func (h *Hub) IsUserOnline(userID string) bool {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	return len(h.UserClients[userID]) > 0
}
