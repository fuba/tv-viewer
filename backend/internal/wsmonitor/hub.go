package wsmonitor

import (
	"encoding/json"
	"log"
	"sync"
	"time"
)

const (
	// Time to wait after disconnection before stopping session
	DisconnectTimeout = 30 * time.Second
)

// SessionStopper is an interface for stopping encoding sessions
type SessionStopper interface {
	StopEncoding(channelID string)
	GetActiveSessions() []string
	GetSessionInfo(channelID string) (map[string]interface{}, error)
}

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	// Registered clients by sessionID
	clients map[string]*Client

	// Register requests from clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Reference to session stopper (encoder)
	sessionStopper SessionStopper

	// Mutex for thread-safe operations
	mu sync.RWMutex

	// Disconnect timers for sessions
	disconnectTimers map[string]*time.Timer
}

// NewHub creates a new Hub instance
func NewHub(stopper SessionStopper) *Hub {
	return &Hub{
		clients:          make(map[string]*Client),
		register:         make(chan *Client),
		unregister:       make(chan *Client),
		sessionStopper:   stopper,
		disconnectTimers: make(map[string]*time.Timer),
	}
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.handleRegister(client)

		case client := <-h.unregister:
			h.handleUnregister(client)
		}
	}
}

// Register adds a client to the hub
func (h *Hub) Register(client *Client) {
	h.register <- client
}

func (h *Hub) handleRegister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	sessionID := client.sessionID
	log.Printf("[wsmonitor] Client registered for session: %s", sessionID)

	// Cancel any pending disconnect timer
	if timer, exists := h.disconnectTimers[sessionID]; exists {
		timer.Stop()
		delete(h.disconnectTimers, sessionID)
		log.Printf("[wsmonitor] Cancelled disconnect timer for session: %s (client reconnected)", sessionID)
	}

	// Close existing client for same session if any
	if existing, exists := h.clients[sessionID]; exists {
		close(existing.send)
		existing.conn.Close()
	}

	h.clients[sessionID] = client

	// Send acknowledgment
	h.sendToClient(client, ServerMessage{
		Type:      MsgTypeSessionStatus,
		SessionID: sessionID,
		Status:    "active",
		Message:   "Connected to session monitor",
		Timestamp: time.Now().UnixMilli(),
	})
}

func (h *Hub) handleUnregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	sessionID := client.sessionID

	if _, exists := h.clients[sessionID]; exists {
		delete(h.clients, sessionID)
		close(client.send)
		log.Printf("[wsmonitor] Client unregistered for session: %s", sessionID)

		// Start disconnect timer
		h.startDisconnectTimer(sessionID)
	}
}

func (h *Hub) startDisconnectTimer(sessionID string) {
	// Cancel existing timer if any
	if timer, exists := h.disconnectTimers[sessionID]; exists {
		timer.Stop()
	}

	log.Printf("[wsmonitor] Starting %v disconnect timer for session: %s", DisconnectTimeout, sessionID)

	h.disconnectTimers[sessionID] = time.AfterFunc(DisconnectTimeout, func() {
		h.mu.Lock()
		delete(h.disconnectTimers, sessionID)
		h.mu.Unlock()

		// Check if client reconnected
		h.mu.RLock()
		_, reconnected := h.clients[sessionID]
		h.mu.RUnlock()

		if !reconnected {
			log.Printf("[wsmonitor] Disconnect timeout reached for session: %s - stopping encoding", sessionID)
			h.stopSessionByID(sessionID)
		}
	})
}

func (h *Hub) stopSessionByID(sessionID string) {
	if h.sessionStopper == nil {
		log.Printf("[wsmonitor] No session stopper configured, cannot stop session: %s", sessionID)
		return
	}

	// Get active sessions and find matching one
	activeSessions := h.sessionStopper.GetActiveSessions()

	for _, channelID := range activeSessions {
		// Check if this channel's session matches
		info, err := h.sessionStopper.GetSessionInfo(channelID)
		if err == nil {
			if sid, ok := info["session_id"].(string); ok && sid == sessionID {
				log.Printf("[wsmonitor] Stopping encoding for channel %s (session: %s)", channelID, sessionID)
				h.sessionStopper.StopEncoding(channelID)
				return
			}
		}
	}

	// If sessionID matches channelID directly (for service streams like SVC_xxx)
	for _, channelID := range activeSessions {
		if channelID == sessionID {
			log.Printf("[wsmonitor] Stopping encoding for session: %s", sessionID)
			h.sessionStopper.StopEncoding(sessionID)
			return
		}
	}

	log.Printf("[wsmonitor] Session %s not found in active sessions, may have already stopped", sessionID)
}

func (h *Hub) processMessage(client *Client, rawMessage []byte) {
	var msg ClientMessage
	if err := json.Unmarshal(rawMessage, &msg); err != nil {
		log.Printf("[wsmonitor] Failed to parse WebSocket message: %v", err)
		return
	}

	switch msg.Type {
	case MsgTypePing:
		h.sendToClient(client, ServerMessage{
			Type:      MsgTypePong,
			SessionID: client.sessionID,
			Timestamp: time.Now().UnixMilli(),
		})

	case MsgTypeStatus:
		// Return current session status
		status := "unknown"
		if h.sessionStopper != nil {
			sessions := h.sessionStopper.GetActiveSessions()
			for _, s := range sessions {
				if info, err := h.sessionStopper.GetSessionInfo(s); err == nil {
					if sid, ok := info["session_id"].(string); ok && sid == client.sessionID {
						status = "active"
						break
					}
				}
			}
		}
		h.sendToClient(client, ServerMessage{
			Type:      MsgTypeSessionStatus,
			SessionID: client.sessionID,
			Status:    status,
			Timestamp: time.Now().UnixMilli(),
		})
	}
}

func (h *Hub) sendToClient(client *Client, msg ServerMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[wsmonitor] Failed to marshal message: %v", err)
		return
	}

	select {
	case client.send <- data:
	default:
		// Client buffer full, close connection
		h.mu.Lock()
		delete(h.clients, client.sessionID)
		close(client.send)
		h.mu.Unlock()
	}
}

// NotifySessionStopped notifies connected client that session was stopped
func (h *Hub) NotifySessionStopped(sessionID string) {
	h.mu.RLock()
	client, exists := h.clients[sessionID]
	h.mu.RUnlock()

	if exists {
		h.sendToClient(client, ServerMessage{
			Type:      MsgTypeSessionStatus,
			SessionID: sessionID,
			Status:    "stopped",
			Message:   "Session has been stopped",
			Timestamp: time.Now().UnixMilli(),
		})
	}
}

// GetConnectedSessionCount returns the number of active WebSocket connections
func (h *Hub) GetConnectedSessionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// GetConnectedSessions returns a list of session IDs with active WebSocket connections
func (h *Hub) GetConnectedSessions() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	sessions := make([]string, 0, len(h.clients))
	for sessionID := range h.clients {
		sessions = append(sessions, sessionID)
	}
	return sessions
}

